#!/usr/bin/env bash
# jsprobe_cmp.sh —— 长输出探针的 wbui / Edge 逐行对比（证据落盘）。
#
# 与 gprobe_cmp.sh 的差别只在 wbui 侧读回方式：
#   gprobe_cmp.sh  用 webshot 的 -js，结果被 summarizeJS 截到 400 字符，只能「每批
#                  3 行」反复 go run（交叉扫描表 500+ 行 ⇒ 170+ 次 go run，且任一
#                  批被截断即静默丢尾部结果）。
#   本脚本         用 dev/tools/jsread 一次读回完整文本，不截断。
# Edge 侧与 gprobe_cmp.sh 完全同口径：--dump-dom + 视口补偿 + python 提取 #out。
#
# 用法（仓库根）：
#   dev/tools/jsprobe_cmp.sh dev/fixtures/webshot/<探针>.html [输出名] [元素id]
#
# 产物（与 gprobe_cmp.sh 同目录同命名）：
#   dev/output/wbui-audit/<名>.wbui.txt / .edge.txt / .cmp.txt / .edge.html
#
# ★ 假 IDENTICAL 防护（同 gprobe_cmp.sh）：任一侧 0 行 → 退出 2；diff 为空但行数
#   不等 → 退出 3。任何「IDENTICAL」都必须伴随两侧非空且行数相等。
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export CGO_ENABLED=1
export PATH="/f/syproject/goskia/bin:$PATH"

PROBE="${1:?用法: dev/tools/jsprobe_cmp.sh <probe.html> [名称] [元素id]}"
NAME="${2:-$(basename "$PROBE" .html)}"
ELEMID="${3:-out}"
OUTDIR="dev/output/wbui-audit"
TMPDIR="dev/output/tmp"
mkdir -p "$OUTDIR" "$TMPDIR"
EDGE="${WBUI_EDGE:-C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe}"

echo "== [$NAME] wbui (jsread 一次读回) =="
go run ./dev/tools/jsread -html "$PROBE" \
  -js "document.getElementById('$ELEMID').textContent" \
  -out "$OUTDIR/$NAME.wbui.txt" | tail -1

echo "== [$NAME] Edge (--dump-dom) =="
# 视口口径补偿：Edge 在 Windows 无头下 --window-size=W,H 的实际视口小于 W,H
# （本机实测恒定非客户区偏移，且不传时默认仅 754x487）。按实测偏移反解补偿，
# 最多 3 轮；探测失败即退出，绝不带错误口径继续比对（与 gprobe_cmp.sh 一致）。
VIEWPORT_W=1280
VIEWPORT_H=800
VP_PROBE="$TMPDIR/_vp_probe.html"
cat > "$VP_PROBE" <<'HTMLEOF'
<!doctype html><meta charset="utf-8"><pre id="vpo"></pre><script>
document.getElementById('vpo').textContent=innerWidth+'x'+innerHeight;
</script>
HTMLEOF
edge_inner() {
  "$EDGE" --headless --disable-gpu --no-sandbox --hide-scrollbars \
    --window-size="$1" --force-device-scale-factor=1 \
    --virtual-time-budget=1500 --dump-dom \
    "file:///$(cygpath -m "$ROOT/$VP_PROBE")" 2>/dev/null \
    | sed -n 's/.*<pre id="vpo">\([0-9][0-9]*x[0-9][0-9]*\)<\/pre>.*/\1/p' | head -1
}
EDGE_WS="${VIEWPORT_W},${VIEWPORT_H}"
vp_try=1
while [ "$vp_try" -le 3 ]; do
  vp_got="$(edge_inner "$EDGE_WS")"
  case "$vp_got" in
    *x*) ;;
    *) echo "[$NAME] Edge 视口探测失败（得到 '$vp_got'），拒绝带错误口径比对" >&2; exit 4 ;;
  esac
  vp_iw="${vp_got%x*}"; vp_ih="${vp_got#*x}"
  if [ "$vp_iw" -eq "$VIEWPORT_W" ] && [ "$vp_ih" -eq "$VIEWPORT_H" ]; then break; fi
  EDGE_WS="$((VIEWPORT_W * 2 - vp_iw)),$((VIEWPORT_H * 2 - vp_ih))"
  vp_try=$((vp_try + 1))
done
echo "     Edge 视口实测 $(edge_inner "$EDGE_WS")（目标 ${VIEWPORT_W}x${VIEWPORT_H}；--window-size=$EDGE_WS）"
"$EDGE" --headless --disable-gpu --no-sandbox --hide-scrollbars \
  --window-size="$EDGE_WS" --force-device-scale-factor=1 \
  --virtual-time-budget=3000 --dump-dom \
  "file:///$(cygpath -m "$ROOT/$PROBE")" \
  > "$OUTDIR/$NAME.edge.html" 2>/dev/null

python - "$OUTDIR/$NAME.edge.html" "$ELEMID" "$OUTDIR/$NAME.edge.txt" <<'PY'
import sys, re, html
src, elemid, dst = sys.argv[1], sys.argv[2], sys.argv[3]
doc = open(src, encoding='utf-8', errors='replace').read()
m = re.search(r'<[a-zA-Z]+[^>]*\bid="%s"[^>]*>(.*?)</[a-zA-Z]+>' % re.escape(elemid), doc, re.S)
txt = html.unescape(m.group(1)) if m else ''
# newline='' 必须：Windows 文本模式会把 '\n' 写成 '\r\n'，让 Edge 侧整份变 CRLF
# 而 wbui 侧是 LF —— diff 会全是假差异。
open(dst, 'w', encoding='utf-8', newline='').write(txt.rstrip('\n') + '\n' if txt else '')
PY

echo "== [$NAME] 归一（CRLF→LF + 末尾换行统一）=="
python - "$OUTDIR/$NAME.edge.txt" "$OUTDIR/$NAME.wbui.txt" <<'PY'
import sys
for p in sys.argv[1:]:
    with open(p, encoding='utf-8', errors='replace', newline='') as f:
        s = f.read()
    s = s.replace('\r\n', '\n').replace('\r', '\n').rstrip('\n')
    with open(p, 'w', encoding='utf-8', newline='') as f:
        f.write(s + '\n' if s else '')
PY

echo "== [$NAME] diff =="
WB_LINES=$(wc -l < "$OUTDIR/$NAME.wbui.txt" | tr -d ' ')
ED_LINES=$(wc -l < "$OUTDIR/$NAME.edge.txt" | tr -d ' ')
if [ "$WB_LINES" -eq 0 ] || [ "$ED_LINES" -eq 0 ]; then
  echo "[$NAME] 抓取失败：wbui=$WB_LINES 行 / Edge=$ED_LINES 行（拒绝产出 diff，防假 IDENTICAL）" >&2
  rm -f "$OUTDIR/$NAME.cmp.txt"
  exit 2
fi
if diff -u --strip-trailing-cr "$OUTDIR/$NAME.edge.txt" "$OUTDIR/$NAME.wbui.txt" > "$OUTDIR/$NAME.cmp.txt"; then
  if [ "$WB_LINES" -ne "$ED_LINES" ]; then
    echo "[$NAME] 假 IDENTICAL：diff 为空但行数不等（wbui=$WB_LINES / Edge=$ED_LINES）" >&2
    exit 3
  fi
  echo "IDENTICAL（Edge 基线 == wbui；wbui=$WB_LINES 行 / Edge=$ED_LINES 行）"
else
  echo "DIFF（- Edge / + wbui），共 $(grep -c '^[-+][^-+]' "$OUTDIR/$NAME.cmp.txt") 行差异（wbui=$WB_LINES 行 / Edge=$ED_LINES 行）"
  cat "$OUTDIR/$NAME.cmp.txt"
fi
