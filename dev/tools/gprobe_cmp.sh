#!/usr/bin/env bash
# gprobe_cmp.sh —— H 类验收工具：把探针页面在 **wbui** 与 **Edge** 下渲染后的 DOM 文本
# 逐字对比并落盘（不只是口头声称）。
#
# 用法（仓库根）：
#   dev/tools/gprobe_cmp.sh dev/fixtures/webshot/g6_supports.html [输出名] [元素id]
#     - 输出名缺省 = 探针文件名（不含 .html）
#     - 元素id 缺省 = out（探针把结果写进 #out）
#
# 产物（证据落盘）：
#   dev/output/wbui-audit/<名>.wbui.txt   wbui 读回的 DOM 文本
#   dev/output/wbui-audit/<名>.edge.txt   Edge 读回的 DOM 文本
#   dev/output/wbui-audit/<名>.cmp.txt    逐行 diff（空 = 完全一致）
#   dev/output/wbui-audit/<名>.edge.html  Edge --dump-dom 原始产物
#
# 实现要点：webshot 的 [js] 打印被 summarizeJS 截到 400 字符，故按 3 行一批、
# 用 U+0001 作为批内分隔符分多次读回，最后还原成完整文本。
#
# ★ P2-1（假 IDENTICAL 防护）：wbui 侧首批 `go run` 编译超时会让 total=0，
#   while 不执行 → wbui.txt 为空；旧版 diff 不报错也不等于「一致」，极端时
#   两侧都空会产出空 diff 冒充 IDENTICAL。现在硬性断言：任一侧 0 行 → 退出 2；
#   diff 为空但行数不等 → 退出 3。**任何「IDENTICAL」都必须伴随两侧非空且行数相等。**
set -u
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export CGO_ENABLED=1
export PATH="/f/syproject/goskia/bin:$PATH"

PROBE="${1:?用法: dev/tools/gprobe_cmp.sh <probe.html> [名称] [元素id]}"
NAME="${2:-$(basename "$PROBE" .html)}"
ELEMID="${3:-out}"
OUTDIR="dev/output/wbui-audit"
TMPDIR="dev/output/tmp"
mkdir -p "$OUTDIR" "$TMPDIR"
EDGE="${WBUI_EDGE:-C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe}"

echo "== [$NAME] wbui (webshot) =="
# 先读总行数，再按需分批：webshot 的 [js] 打印被 summarizeJS 截到 400 字符，
# 而探针单行可达 ~100 字符，批越大越可能被截断导致尾部结果丢失。此处每批 3 行。
total="$(go run ./dev/probes/webshot -html "$PROBE" -out "$TMPDIR/$NAME.wbui.png" \
    -js "var e=document.getElementById('$ELEMID'); e?e.textContent.split('\n').length:0" \
    2>/dev/null | sed -n 's/^\[js\] => //p')"
case "$total" in ''|*[!0-9]*) total=0 ;; esac
i=0
while [ "$i" -lt "$total" ]; do
  chunk="$(go run ./dev/probes/webshot -html "$PROBE" \
      -out "$TMPDIR/$NAME.wbui.png" \
      -js "var e=document.getElementById('$ELEMID'); e?e.textContent.split('\n').slice($i,$((i+3))).join('\u0001'):''" \
      2>/dev/null | sed -n 's/^\[js\] => //p')"
  [ -n "$chunk" ] && printf '%s\n' "$chunk" | sed -e 's/\\n/\n/g' -e 's/\x01/\n/g'
  i=$((i+3))
done > "$OUTDIR/$NAME.wbui.txt"

echo "== [$NAME] Edge (--dump-dom) =="
# ★ 视口口径补偿（实测驱动，禁用魔数）：Edge 在 Windows 无头下 `--window-size=W,H`
#   得到的**实际视口小于 W,H** —— 本机实测为恒定的非客户区偏移（宽 −26 / 高 −93），
#   且**不传 --window-size 时默认仅 754x487**（这正是此前 g4_inlineblock 读到 754 的
#   唯一来源）。而 webshot 的 -w/-h 就是精确的 CSS 视口（默认 1280x800）。不补偿会让
#   「块宽=视口宽」的夹具（g4 的 .line 未设 width）在两侧出现整批假差异 —— 曾据此把
#   g4 误判成 WONTFIX「口径差」。做法：先实测当前偏移 → 按 offset 放大 --window-size
#   反向补偿 → 迭代确认实测视口恰为目标值（最多 3 轮；探测失败即退出，绝不带错误
#   口径继续比对）。
#   --force-device-scale-factor=1 额外锁死 DPR，防宿主高 DPI 再叠一层口径差
#   （CSS 视口 = 物理宽 / DPR）。
VIEWPORT_W=1280
VIEWPORT_H=800
VP_PROBE="$TMPDIR/_vp_probe.html"
cat > "$VP_PROBE" <<'HTMLEOF'
<!doctype html><meta charset="utf-8"><pre id="vpo"></pre><script>
document.getElementById('vpo').textContent=innerWidth+'x'+innerHeight;
</script>
HTMLEOF
edge_inner() {  # $1 = window-size；回显实际 "宽x高"
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
# 末尾补一个换行，避免 Edge 侧缺尾换行时 diff 把整份判成差异。
# newline='' 是必须的：Windows 上 Python 文本模式默认把 '\n' 写成 '\r\n'，
# 会让 Edge 侧整份变 CRLF 而 wbui 侧是 LF —— 于是 diff 全是假差异。
open(dst, 'w', encoding='utf-8', newline='').write(txt.rstrip('\n') + '\n' if txt else '')
PY

echo "== [$NAME] 归一（CRLF→LF + 末尾换行统一 + 去空行）=="
python - "$OUTDIR/$NAME.edge.txt" "$OUTDIR/$NAME.wbui.txt" <<'PY'
import sys
# 两侧统一口径：CRLF/CR → LF；去掉末尾所有空行后，各补且仅补一个 '\n'。
# 这样 *.cmp.txt 非空就只代表真实内容差异，而不是换行噪声。
for p in sys.argv[1:]:
    with open(p, encoding='utf-8', errors='replace', newline='') as f:
        s = f.read()
    s = s.replace('\r\n', '\n').replace('\r', '\n').rstrip('\n')
    with open(p, 'w', encoding='utf-8', newline='') as f:
        f.write(s + '\n' if s else '')
PY

echo "== [$NAME] diff =="
# ── P2-1：两侧非空 + 行数一致断言（详见文件头说明）──
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
