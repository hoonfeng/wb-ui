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
# 实现要点：webshot 的 [js] 打印被 summarizeJS 截到 400 字符，故按 6 行一批、
# 用 U+0001 作为批内分隔符分多次读回，最后还原成完整文本。
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
# 先读总行数，再按需分批 —— 每批 3 行：webshot 的 [js] 打印被 summarizeJS 截到
# 400 字符，而探针单行可达 ~100 字符（g2/g7 的 rect + 多属性行），6 行一批会被
# 截断导致尾部结果丢失。
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
done | sed '/^$/d' > "$OUTDIR/$NAME.wbui.txt"

echo "== [$NAME] Edge (--dump-dom) =="
"$EDGE" --headless --disable-gpu --no-sandbox --hide-scrollbars \
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
open(dst, 'w', encoding='utf-8').write(txt.rstrip('\n') + '\n' if txt else '')
PY

echo "== [$NAME] diff =="
if diff -u "$OUTDIR/$NAME.edge.txt" "$OUTDIR/$NAME.wbui.txt" > "$OUTDIR/$NAME.cmp.txt"; then
  echo "IDENTICAL（Edge 基线 == wbui）"
else
  echo "DIFF（- Edge / + wbui），共 $(grep -c '^[-+][^-+]' "$OUTDIR/$NAME.cmp.txt") 行差异"
  cat "$OUTDIR/$NAME.cmp.txt"
fi
