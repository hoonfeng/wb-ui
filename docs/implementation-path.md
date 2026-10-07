# wb-ui 实现路径（v1 · 三线总纲：媒体真实播放 / CDP 调试协议 / JS 引擎对标）

> 状态：**P0/P1 与 P2 全部落地，P3 的 A3 音频后端已落地**（A1/A2 全项、**A3 音频 L1→L4（含 `data:` 来源）**、B2（CDP S2）、C-P2（后端接口抽象）、A4 动图）（2026-10-07 落地 A0 媒体元数据、B0+B1 CDP 调试服务端、C-P0 基线固化，
> 同日续做 **A1 视频出画面（宿主注入帧流）**、**A2 帧推进（L4 起点）**、
> **A2 异步预取（渲染线程不再等解码）**，以及 **A2 收尾四项**：
> 精确到帧的 seek、帧率驱动的预取窗口、`requestVideoFrameCallback`（含帧就绪重绘）、
> TC-M-507 连续帧采样，以及 **B2 CDP S2**：Runtime `objectId` 句柄表（getProperties /
> callFunctionOn(objectId) / releaseObject）、DOM 编辑与查询扩展、**CSS 域**、**console 分级**，
> **C-P2 后端接口抽象**（jsc 的 goja 耦合收敛到单文件 + `Backend` 契约与注册表），
> 以及 **A4 动图**（goskia 暴露 `SkCodec` 多帧 + 引擎按帧时长推进）；
> 以及 **A3 遗留清理**（`data:` 内联字节先落盘再交给 ffmpeg：音频/视频 data: 共 32 格 L1→L4）；
> 逐项证据见 [§0.1](#01-实装进度2026-10-07-更新)）。设计稿阶段为 2026-10-06。
> 定位：把三条「需要真实实施（写引擎代码）」的主线收进同一份分期表，避免各写各的。
> 验证与实现分离：**媒体能力等级的验证**属独立媒体验证项目（设计稿 [docs/media-format-verification-plan.md](media-format-verification-plan.md)），
> 本文只负责**实现路径**（要写哪些代码、按什么顺序、怎么验收）。
>
> 本文所有「现状」结论均经本轮实测/取证（证据行号见文末附录），不做未验证的推断。

---

## 0.1 实装进度（2026-10-07 更新）

**已落地并验收**（每条都有可执行证据，不是「写完了」的自我声明）：

| 期 | 交付 | 落点 | 验收证据 |
|---|---|---|---|
| P0 | **A0 媒体元数据注入** | `engine/js/bindings/media_element.go`（`MediaMetadata{Duration,Width,Height}` + resolver 签名升级）、`app/mediaprobe.go`（`ffmpeg -i` 解析 + 缓存）、`app/host.go` 与 `cmd/psai` 注入 | `_temp/psai.exe -verify` 媒体段：`readyState=4`、`duration=1`、`videoWidth×Height=120x80`、事件序列 `loadstart→durationchange→loadedmetadata→loadeddata→canplay`；`app/mediaprobe_test.go`（含真实 ffmpeg 生成样本的端到端） |
| P1 | **A1 视频出画面（宿主注入帧流）** | 新 `engine/rendering/videoframe.go`（帧源注册 + 元素显示状态 + `(url,时刻)` 帧缓存/负缓存）；`engine/rendering/painter.go`（poster frame 二选一）；`engine/js/bindings/media_element.go`（`showPoster` flag 与显示状态同步）；`app/mediaprobe.go`（`ffmpeg -ss … -frames:v 1` 抽帧 + 缓存 + 注册帧源） | `_temp/psai.exe -verify` 判据 A1-1（无 poster 的 `<video>` 像素 = 独立 ffmpeg 抽帧的参照值）、A1-2（有 poster 未播放时显示 poster）、A1-3（seek 到 0.5s 后 poster 让位给帧）全过；`engine/rendering` 7 个单测、`engine/js/bindings` 7 个单测 |
| P2 | **A2 全部**（帧推进 + 异步预取/帧队列 + 精确到帧的 seek + 帧率预取窗口 + `requestVideoFrameCallback`） | `app/mediaprobe.go` 的 `MediaProbe.clampFrameTime`（越界时刻收敛到 `duration - 0.1s`，见实现事实 11）、**`alignFrameTime` 帧对齐 + `MediaProbe.FPS` + `FrameTimeQuantizer`**（见实现事实 18/19）；**异步抽帧** `app/mediaframepump.go`（有界队列 + 2 个 worker，惰性启动/空闲自退，队满丢最旧且**也要交付失败**）+ `engine/rendering/videoframe.go` 的 `VideoAsyncFrameSource` / `VideoElementState.Playing` / `PrefetchVideoFrame` / `VideoFrameStats` / `SetVideoFrameTimeQuantizer` / `AddVideoFrameReadyListener`；绑定层每次 tick **按帧**预取一个窗口（`media_element.go` 的 `prefetchNextFrame`）与 `requestVideoFrameCallback`；`webkit` 的 `onAsyncVideoFrame`（帧到位 → 置脏重绘）；`cmd/psai/mediaframe_selftest.go`（两段式 + **每帧一色**样本、播放推进自检、收敛窗口采样、逐帧参照 `-vf select=eq(n,k)`） | 判据 A2-1（播放中画面 = 该时刻参照帧）、A2-2（结束后画面 = 末尾参照**且** ≠ 播放前首帧）、A2-3（事件序列 `play→playing→timeupdate×4→ended`）、**A2-4（播放全程同步抽帧 0 次：预取 8 次、异步交付 9 帧、回退上一帧 6 次）**、**A2-5（20 个采样点：每帧区间前段/后段画面都 = 该时刻所属帧）**、**A2-6（连续采样 5 帧：帧号 [0 1 2 3 4]，每帧都 = 该帧参照）**、**A2-7（播放中 rVFC 回调 4 次：presentedFrames 1..4 逐个递增、mediaTime 0.2→1、尺寸 120x80）** 全过；`engine/rendering` 异步测试（含 `-race`）、`engine/js/bindings` 的 rVFC/预取测试、`webkit` 的 `TestVideoFrameDeliveryMarksFrameDirty`、`app` 的 `TestFramePump*` / `TestAlignFrameTime` / `TestMediaProbeFrameAtExactFrame` |
| P0 | **B0 CDP 最小服务端** | 新包 `engine/devtools/cdp`（`ws` 子包 = RFC6455 子集；JSON-RPC 分派；Target/Session；Browser/Target/Runtime/Page 域）、`app/cdp.go` 适配层、宿主开关 `--remote-debugging-port` | 判据 1/2/3/7 全过（[docs/CDP.md](CDP.md) §5）；`engine/devtools/cdp` 与 `cdp/ws` 共 19 个单测 |
| P1 | **B1 CDP 可用（S1 场景）** | `Page.captureScreenshot`、`Input.*`、`DOM.*`（读）、`Emulation.setDeviceMetricsOverride`、事件泵（console / log / 页面加载） | 判据 4（截图与 `Render()` 逐像素一致：1296000 个不透明像素全等）、5（CDP 点击 → 目标元素 click 计数 = 1，且页面向宿主发出 `tool.select`）、6（DOM 节点数 155 = 页面计数 155）全过；**外部 Python 客户端**实测驱动通过 |
| P2 | **B2 CDP S2（DevTools Elements 所需域）** | Runtime 的 **`objectId` 句柄表**（`app/cdp.go` 的页面侧 `__cdpHandles` 表 + `EvaluateHandle` / `GetProperties` / `CallFunctionOnObject` / `ReleaseObject`）；DOM 扩展（`querySelectorAll` / `getAttributes` / `getOuterHTML` / `resolveNode` / `setAttributeValue` / `removeAttribute` / `requestChildNodes`）；**CSS 域**（`enable`/`disable` + `getComputedStyleForNode` / `getInlineStylesForNode` / `getMatchedStylesForNode`）；**console 分级**（`jsc.BufferLogger` 加 `Entries`（Level+Text），`console.warn/error/info/debug` 按级别落账 → `Log.entryAdded` / `Runtime.consoleAPICalled` 保真） | 判据 **8（对象求值 → objectId=cdp:0；getProperties 列出 a/b；callFunctionOn(this.b)=「两」；releaseObject 成功）**、**9（querySelectorAll('div') → 36 个节点；getAttributes 4 项；outerHTML 11937 字节；resolveNode → objectId）**、**10（setAttributeValue → 页面读到 42；removeAttribute → 属性消失）**、**11（computedStyle 26 项 / inlineStyle 2 项 / matchedCSSRules 1 条）**、**12（console.error/warn → Log.entryAdded level=error/warn）** 全过 |
| P0 | **C-P0 基线固化** | `dev/probes/jsesmatrix`（以 `featuresBlackList` 为权威清单，运行时解析 → 不手抄；缺项即报错「不允许未知」；`-v` 逐项、`-out` 快照） | 45 项：supported 7（含引擎自补的 `WeakRef`/`FinalizationRegistry`）/ missing 35（其中 5 项已排期）/ exempt 3；快照 `dev/output/jsesmatrix.json` |
| P2 | **C-P2 后端接口抽象** | `engine/js/jsc/backend.go`：`Backend` / `RuntimeHandle` 契约 + 后端注册表（`RegisterBackend` / `SetActiveBackend` / `ActiveBackend` / `BackendNames`）；`engine/js/jsc/backend_goja.go`：**全包唯一 import goja 的文件**——goja 的类型与构造器收敛为 `be*` 名字（`beValue` / `beObject` / `beRuntime` / `beFunctionCall` / `beCallable` / …）+ goja 后端注册；`goja_adapter` / `streams` / `webapi` / `env` / `lazy` / `eventloop` 六个文件改为只用 `be*` | `grep -l 'engine/js/goja"' engine/js/jsc/*.go` **只剩 `backend_goja.go`（+ 测试）**；`bindings` **零改动**（`grep 'beValue\|beObject' engine/js/bindings` 为空）；`GOWORK=off go build ./...` + `go test ./engine/js/jsc ./engine/js/bindings ./engine/js/worker ./app` 全绿；`_temp/psai.exe -verify` 仍 **28 ✓ / 0 ✗** |
| P2 | **A4 动图（GIF/WebP 多帧）** | **goskia**：新 `skia/codec.go`（`NewCodec` / `FrameCount` / `Dimensions` / `FrameDurationMS` / `RepetitionCount` / `DecodeFrame` / `DecodeFrames`，`fPriorFrame = i-1` 处理增量帧）+ `skia/codec_test.go`（3 帧 GIF 逐帧像素不同、静态 PNG 走单帧）；**wb-ui**：`engine/rendering/imageanimation.go`（`AnimatedImageSource` 注入 + 帧序列缓存 + `HasAnimatedImages` + 假时钟可测）、`engine/rendering/image.go` 的 `NewDecodedImageFromSkia`（直接持有帧位图，不走 PNG 中转）、`painter.go` 的 `<img>` 每帧重取当前帧、`backgroundimage.go` 的四个解码点接入（动图**不进单帧缓存**）、`app/imageanimation.go`（宿主用 SkCodec 解多帧）、`app/host.go` 的 `needPaint` 加 `HasAnimatedImages()` | **判据 A4-1（6 次采样、每帧 300ms：出现 3 种帧色，画面随帧推进变化且都等于样本帧色）**；单测：goskia `TestCodec*`（2 项）、`engine/rendering` 的 `TestAnimated*`（3 项：按时钟推进 / 非动图回退单帧 / `loops=0` 停末帧）、`app` 的 `TestAnimatedImageSourceDecodesGIFAndAdvances`（宿主源 + 引擎通道的集成） |
| P3 | **A3 音频输出后端（L4-S）+ `data:` 来源闭环** | 宿主 `app/mediaaudio.go`（ffmpeg 解 s16le PCM）、`app/audioout_windows.go`（waveOut，位置查询用 `TIME_BYTES=0x0004`）、`app/audioout_other.go`（非 Windows 挂钟降级——播放不停摆、只降时钟精度）、引擎 `engine/rendering/audioframe.go`（PCM 注入通道）+ `engine/js/bindings/mediaaudio.go`（会话与「**输出位置**驱动」的时钟）；`data:` 来源由新 `app/mediadataurl.go` 落盘成普通本地文件（`mediaSrcToPath` 接线，见实现事实 27） | 音频 `sine-440-1s.{wav,mp3,ogg,m4a}` × `file\|rel\|data` × 四配置 = **48 格 L4**（判据 A 主峰 439.88/439.87Hz、幅度 1.000；TC-M-602 终态 1.00s；判据 B 环回录音按统一口径如实记录（无设备时记 `SKIP(no-loopback)`、有候选但未路由时记 `mismatch`，见 `media-format-verification-plan.md` §9.6））；`data:` 闭环使音频 16 格 L1→L4、视频 data: 16 格同步恢复（**32 上升 / 0 下降**，随后连续两次「与基线一致」）；单测 `app/mediaaudio_test.go`（判据 A + 设备位置推进/`flush` 归零）、`app/mediadataurl_test.go`（含 `TestMediaDataURLFeedsFFmpeg` 端到端） |

**尚未落地**（按 §6 分期）：

- **P2**：**已全部落地**（A2 全项、A4 动图、B2（CDP S2）、C-P2 后端接口抽象，见上表）。
- **P3（需用户确认）**：B3 `Target` 扁平会话 + `Emulation` + `Page.navigate` → S3、C-P3 V8 后端（+60MB 分发）、C-P4 跨界优化。
  （**A3 音频后端已落地**（2026-10）：宿主注入 PCM + waveOut 输出；`data:` 来源亦已闭环——音频 `file|rel|data` 全 **L4**，见 §2 的 A3 行与 [media-format-verification-plan.md](media-format-verification-plan.md) §9.6/§9.8。）

**本轮新增的实现事实（原设计稿未预见，后续改动请勿踩回）**：

1. `MediaMetadataResolver` 只回时长不够——`videoWidth/videoHeight` 在 L1 就该可用，签名升级为返回 `MediaMetadata`；「探测到尺寸但时长未知」时停在 `HAVE_METADATA`，**不派发 canplay**（没有时长就不该声称能播放）。
2. 无头自检此前**从不推进事件循环的宏任务**（`cmd/psai` 的 `settle` 只 `Render`）——媒体事件、延迟回调整批缺失；已对齐宿主的每帧行为（`ProcessTasks` + `RunJobs`）。
3. `cmd/psai` 的 `fileURLOf` 对相对路径产出 `file:///_temp/…`（缺盘符）→ 已改为先绝对化。
4. 引擎的 console 管道**不区分级别**（`log`/`warn`/`error` 走同一个写入函数）→ CDP 侧一律按 `log` 上报（S2 待办，见 TECH_DEBT）。
5. Windows 上端口处于 `TIME_WAIT` 时无法立即重绑 → 宿主错误信息给出「换端口」提示（不改监听套接字选项：`SO_REUSEADDR` 在 Windows 上语义危险）。
6. `jsesmatrix` 必须跑在 `jsc.RegisterWebAPIs()` 之后的引擎环境，否则会把「引擎自补的能力」误报成缺失；它给出的是**粗粒度能力探测**，不等于 tc39 逐用例一致。
7. **`show poster flag` ≠「不许画帧」**：规范里 poster frame 的定义是「有 poster 属性且图可用 + flag 置位 → 那张图；**否则用当前播放位置的帧**；再没有才什么都不画」。所以无 poster 的 `<video>` 加载完必须画出第一帧（不是空白），有 poster 的才先显示 poster、等 `play()` 或 `currentTime` 设为非 0 后切到真实帧。判断入口是 `rendering.videoShowsPoster`（painter 用它二选一）——**不要**把判断写成「帧永远优先」或「poster 永远优先」。
8. **`<video>/<audio>` 的 `src` 不是图片**：painter 以前把 `src`（mp4/wav）交给图片解码器——每次绘制白读一遍文件、再让 Skia 判失败，而且无 poster 的 `<video>` 整条绘制路径不可达。现在画面只来自「宿主注入帧」与「poster」两条（`src` 只用于取 `poster` 属性）。
9. **`data:` URI 必须在模式门禁之前放行**（本轮修复）：`ImageResourceLoader` 的契约本来就写着「data: URL 不走本判定（两种模式都允许）」，但实现把 `AllowsExternal` 门禁放在了 `decodeDataURI` 之前——ModeToolkit 下连 `<video poster="data:…">`、内联 data: 图标都被拒。修复后「data: 无条件放行」的验收（TC-M-901）才成立，A1-2 判据也才能用 data: poster 做无副作用取样。
10. **自检插桩必须自清理**：A1 自检插入的 `<video>` 是 `fixed` + 最高 `z-index`（取样点必须稳定且可复现），会挡住后续判据的点击目标（CDP 判据 5 点的就是页面左上角）——第一版实现直接让判据 5 回归（`click 计数 = 0`）。插桩要 `defer` 移除并 `settle`，否则「自检」本身会污染下一条自检。
11. **播放到结束时必须保留最后一帧**：`currentTime == duration` 这个精确时刻**没有帧**（1s/10fps 的末帧在 0.9s），直接抽帧会空手而归 → 画面在结束时消失。宿主按已探测的时长把越界请求收敛到 `duration - 0.1s`（`MediaProbe.clampFrameTime`），再留「退一步重试」作兜底。★ 实测本机 ffmpeg 的 `-sseof -0.05` 在这些样本上产出 **0 字节**（`-ss 0.9` 才有帧），所以末尾兜底**不要**用 `-sseof`。
12. **帧推进类判据要有「起点对照」**：不能用「两个任意时刻的像素不同」——首版实现里两次采样恰好都落在同一个颜色段（0.5s 与 1.0s 都是后段），判据假失败。正确做法是拿**播放前的首帧**当对照（并先断言它确实等于首帧参照），再断言结束画面与它不同。
13. **「静止态同步取帧、播放推进异步取帧」是刻意的分工**（A2）：painter 在渲染线程上取帧，而抽帧是外部进程调用（几十毫秒）。一律异步则加载首帧要等「下一次重绘」才出现（按需渲染的宿主可能一直不重绘）；一律同步则每换一帧都卡住一次绘制。所以 `VideoElementState.Playing`（绑定层按「时钟是否在走」填）为真时走异步（帧未交付就先显示**上一次成功显示**的帧，且只认同一 src），为假时同步（首帧/seek/暂停必须在这**一次**绘制里画出来）。
14. **`Playing` 的时序要盯三处**：`play()` 必须**先** `startClock()` 再 `syncVideoState()`（否则 play 后到首个 tick 之间的 250ms 仍声称 Playing=false，painter 白白同步等一次解码）；`pause()` 也**必须**上报状态（本轮补的——此前 pause 后 Playing 还是 true，暂停画面不精确）；`tick()` 里写状态与预取都在推进 `currentTime` 之后（预取的是**下一次** tick 的时刻）。
15. **预取超前量 = 一个时钟步长（250ms）**：绑定层每次 tick 预取 `currentTime + 250ms × playbackRate`（末尾收敛到 `duration`——宿主会把越界时刻映射到最后一帧）。这样 painter 到达时通常已命中缓存，「播放中同步抽帧 = 0」（判据 A2-4）才成立。更长的超前窗口需要帧率（宿主目前不提供 fps），见 TECH_DEBT。
16. **异步交付必须「恰好一次」，且要有兜底**：宿主契约是每个请求恰好交付一次；`framePump` 在队列满丢掉最旧任务时**也要**交付失败（漏交会让渲染层一直等那个键）。渲染层再叠一层 `inflightMaxAge = 10s` 的失效——宿主实现不当或漏交时，该时刻的帧仍可重新请求，而不是**永久**停在上一帧。
17. **A2 的判据不能假设「画面与 currentTime 零延迟」**：异步交付有几十毫秒延迟，采样必须给收敛窗口（`mvSampleStable` / `mvSampleUntil`：500ms 内反复重绘 + 重读 `currentTime`），否则会在「时钟刚跨过颜色分界」的瞬间误报失败。判据强度的另一半来自 A2-4（同步抽帧 0 次 = 帧确实是被**提前**备好的，而不是碰巧）。
18. **`ffmpeg -ss` 是 ceil 语义（实测，不是推测）**：`-ss t -i f -frames:v 1` 取的是「时间戳 **≥ t 的第一帧**」——本机实测（每帧一色的 10fps 样本，参照来自按帧号的 `-vf select=eq(n,k)`）：请求 0.05s 得到第 1 帧、0.01s 直接跳过首帧、0.45s 得到第 5 帧。而 HTML 的 `currentTime` 语义是「显示 t **所属**的那一帧」（floor）。⇒ 直接拿 currentTime 抽帧会**整体提前一帧**（A1-3 恰好用 0.5s 这个帧边界，所以此前没暴露）。修正规则：把请求值左移到目标帧区间的取样点 `(k-0.5)/fps`（`app/mediaprobe.go` 的 `alignFrameTime`）——实测输入侧、输出侧、「输入侧预跳 + 输出侧精确」三种调用方式都取到第 k 帧；**帧率未知时不对齐**（偏差 ≤ 一帧，不猜）。
19. **「时刻 → 帧」的折叠必须贯通两层**：帧是离散的而 currentTime 是连续的，同一帧会被问很多个时刻（0.51/0.55/0.59）。宿主 `FrameAt` 在算缓存键**之前**做「收敛 + 对齐」，渲染层注册 `SetVideoFrameTimeQuantizer`（同一套规则）折叠显示与预取的键 ⇒ 同帧只解码一次、只缓存一份，预取与绘制互相命中。★ 两层的折叠规则必须**严格一致**，否则折叠后的键会指到别的帧。
20. **预取窗口的宽度来自帧率**：知道 fps 时按帧预取 `mediaPrefetchFrames`(3) 帧（10fps 下 = 300ms 窗口），未知时退回一个时钟步长；多预取几个时刻**不会**变成多次解码（折叠 + 在飞去重把同一帧算一次）。
21. **`requestVideoFrameCallback` 的「呈现」判定挂在时钟步长上**：本引擎没有合成器，把「新帧呈现」定义为**帧号变化**（`frameNo = floor(currentTime*fps)`，帧率未知退化为毫秒）。★ 帧号跟踪必须与「当前有没有注册回调」**解耦**——否则注册那一刻的 `lastFrameNo` 停在旧帧号上，注册后第一次时钟推进就误报一次「新帧」（首版实现被自己的单测当场抓住）。已知差异（记在 TECH_DEBT）：回调粒度 = 时钟步长（10fps/1s 播放回调 4 次而不是 10 次）、`this` 传 undefined、`presentationTime` 是宿主 wall clock 近似、`processingDuration` 恒 0。
22. **帧到位必须由引擎自己置脏**：异步抽帧的字节在 worker goroutine 上回来，按需渲染的宿主（`rv.IsDirty()` 为假跳过 Paint）不会自己发现帧缓存里多了一帧——`rendering.AddVideoFrameReadyListener` → `webkit.onAsyncVideoFrame` → `MarkRenderTreeDirty + MarkAllDirty`（**不**请求重排：画面不参与布局）。与图片那条通知（`onAsyncImageLoaded`）同一套路，反向验证见 `webkit/videoframe_repaint_test.go`。
23. **`jsc` 的后端名一律 `be*`，goja 只出现在一个文件里**（C-P2）：把 goja 符号收敛到 `backend_goja.go` 的类型别名与包装后，其余六个文件（含引用最密的 `streams.go`）只用 `be*`。★ 因此**新增 JS 引擎相关代码不要再直接 `import goja`**——需要什么能力就在 `backend_goja.go` 加一个 `be*` 包装（换后端时只需提供同名定义，这正是 C-P3 的接入点）。
24. **本仓库 vendored goja 的 `Callable` 签名与上游不同**：这里是 `type Callable func(this Value, args ...Value) (Value, error)`（上游是 `func(FunctionCall) Value`），调用方写 `fn(this, args...)`；`goja.NewString/NewFloat/NewBoolean` 也是**包级**构造器（不接 runtime）。按上游签名写包装会在编译期报「too many arguments」——首版 `be*` 包装正是这样被编译器当场抓住的（别凭记忆写 goja API）。
25. **动图的帧快照会被 RenderBox 固化（A4 的关键坑）**：`PaintImage` 原来只在 `img == nil || !img.Loaded()` 时才加载图片，并用 `box.SetDecodedImage(img)` 把结果存下来——GIF 的**第一帧因此被固化，动图永远不动**（判据 A4-1 首版实测「6 次采样只出现 1 种颜色」）。修正：绘制 `<img>` 时每次都重新问一次「现在该显示哪一帧」（`animatedFrameForSrc`，走与加载时同一套 URL 解析），box 上的缓存只作兜底。★ 同理，**动图不能进 `backgroundImageCache.imgs`**：那张表一个 url 只存一帧，命中即固化。
26. **自检探针资源的两个约束**：① 用 **`data:` URI** 注入（psai 有资源拦截器/宿主 loader，`_temp/` 下的新文件不一定取得到；`data:` 不经 loader，直接进解码路径——A1-2 的 poster 同一思路）；② **要注意判据耗时对后续判据的影响**：A4-1 要连续采样约 2 秒，页面状态在等待期间会推进，先跑它会让 CDP 判据的目标元素（工具条/输入框）不再处于初始状态（判据 5 与键盘项实测直接变成「跳过」）——所以它排在 CDP 判据**之后**。

27. **宿主只认「路径」，`data:` 必须先落盘**（A3 遗留清理）：宿主把解码/抽帧交给 ffmpeg，而 ffmpeg 读的是文件或管道——`data:`（RFC 2397 内联字节）没有路径可给，修复前 `mediaSrcToPath` 对 data: 一律 false（音频 data: 16 格停在 L1、视频 data: 连抽帧都进不去）。修法：`app/mediadataurl.go` 把 data: 解成字节 → 落到系统临时目录，文件名 = **内容摘要（sha256 前 8 字节）+ 按 MIME 推断的扩展名** ⇒ 同一 URI 只落一次、**跨进程**也命中（连续两次全量跑不重写），且路径随内容而定，探测缓存与帧缓存按 `(路径, 时刻)` 建键才不会每帧重解一次 base64。★ 两条纪律：① **不要**在每条链路上各写一套「先解码再喂管道」的分支（一处落盘、三条链路共用）；② 落盘文件**刻意不自动清理**（跨进程复用），需要「跑完不留垃圾」时显式调 `CleanupMediaDataURLFiles()`。

28. **媒体引用的门禁必须与 `<img>` 同源，而且要挡住「宿主自己读盘」这条链**（A3 遗留清理之三，§9.9）：媒体的字节从不进引擎（解码全在宿主），因此它**不走** `loadExternalResource`，`ResourcePurpose` 里也一直没有 `PurposeMedia` ⇒ 媒体链路的策略门禁**完全缺失**：`Toolkit+DenyExternal` 下同一个 `file://` 引用，`<img>`/background 是 L0、`<video>/<audio>` 却是 L4（宿主真的读盘并解码了）。修法（一份判定、两处执行）：`webkit.MediaResourceAllowed`（判定**基元与顺序**与 `loadExternalResource` 同源——resolver 命中 → `data:` → 策略门禁）＋ 引擎侧 `bindings.MediaSrcAllowed`（资源选择 `finishLoad` 最先判定，被拒的引用与 http(s) 不可达走同一条失败路径 `NETWORK_NO_SOURCE` + `error`）＋ 宿主侧 `app/mediaaccess.go` 与 `mediaSrcToPath` 前置门禁（元数据/抽帧/PCM 三条链路共用这一个入口，因此一次拦全）。★ 三条纪律：① **不在媒体链路上散判**（媒体专属规则零条，判定只有一份）；② `data:` 与策略解耦**恒放行**、`http(s)`/`blob` 在媒体通道**恒拒**（后者是**能力边界**——引擎不代宿主联网、宿主也没有网络媒体解码通道，不是策略差异）；③ **需要本地媒体的宿主必须显式声明策略**（`AllowHostResolved` 或 `AllowAll`；`cmd/psai` 的 A0/A1/A2 自检段即显式切 `AllowAll` 跑完再恢复）——安全默认是「宿主没表态就不读盘」，与 U1 的图片口径同一条规矩。

**§8 决策点的执行情况**：决策 2 取推荐 (a) 自研最小 WS；决策 3 取 (b) 后端化（C-P2 未开工，默认后端仍是 goja）；决策 5 取「三线并行、P0 优先」；**A1 的两条路线取路线 1（宿主注入帧流）**——它把解码器留在宿主，引擎不背 ffmpeg 的体积与许可，因此**不受决策 4（分发体积）约束**，可以先落地。决策 1（CDP 目标场景）与决策 4 涉及 P2/P3，**仍待用户拍板**——在此之前不动 V8 后端与音频后端。

---

## 0. 一句话结论

| 主线 | 现状 | 目标 | 主要工作量落在 |
|---|---|---|---|
| **A 媒体真实播放** | 图像解码可用；**视频/音频零能力**（无解码器、无音频后端、宿主未注入元数据解析器） | `<video>`/`<audio>` 能加载、能出画面、能出声（L1→L4） | 宿主注入层 + 音频后端 + goskia（`SkCodec`） |
| **B CDP 调试协议** | **完全没有**（无 WS 服务端、不监听任何调试端口、无协议实现） | 外部工具能连上引擎：`json/version` + WS + 最小域子集 | `engine/devtools/cdp/`（新包）+ 宿主开关 |
| **C JS 引擎对标** | vendored **goja**（纯 Go 字节码解释器，**无 JIT**）；ES 覆盖到 ES2015+ 的大部分，**无 ES 模块**、无 `Temporal`/`Atomics`/`SharedArrayBuffer` 等 | JIT + 完整 ES 对标浏览器 | `engine/js/jsc` 适配层后端化 +（V8 可选后端） |

**关键判定（本轮新增，务必先读）**：

1. **「改造 goja 得到 JIT」不成立**：goja 是**字节码解释器**，上游明确不以 V8/JSC 为对手，也不做 JIT。
   想在 goja 内部「加 JIT」等于重写一个引擎——不是改造，是另起项目。
2. **「完整 ES 对标浏览器」在 goja 上只能靠逐项自研**：缺口有权威清单（`engine/js/goja/tc39_test.go` 的
   `featuresBlackList`，40+ 项，含 ES 模块、`async-iteration`、`Temporal`、`Atomics` 等）。
3. **真正同时满足「JIT + 完整 ES」的只有换/并行一个工业级引擎**，而 V8 路线**本仓库已实测跑通**
   （`scripts/v8/README.md`，2026-09-27），代价是分发体积 +60MB 与 JS↔Go 跨界变慢 7–240×。
   ⇒ 主线 C 的推荐做法不是「二选一」，而是**把 `jsc` 适配层做成可替换后端**：默认 goja，重性能/完整度时切 V8。
4. **CDP 与 JS 引擎无关**：CDP 是**宿主侧的调试服务端**（Go 写的 WS + JSON-RPC），不需要引擎 JS 具备真实
   WebSocket（引擎内的 `WebSocket` 是**宿主注入事件的 stub**，见 `docs/TECH_DEBT.md`「附：WebSocket」）。

---

## 1. 现状基线（实测取证）

### 1.1 媒体（主线 A 的起点）

| 能力 | 现状 | 证据 |
|---|---|---|
| 光栅解码 | PNG/JPEG/GIF/WebP/BMP/ICO 可解码并绘制 | `_temp/mediacheck/out2.txt`（21 样本矩阵） |
| 布局固有尺寸 | 走 Go `image.DecodeConfig`，与 Skia 解码集合**不一致**（WebP/BMP/ICO 缺几何） | 同上 |
| SVG | 自有矢量路径，不入 Skia 解码器 | `engine/rendering/` SVG 管线 |
| 动图 | GIF/WebP 动画**只取首帧**（无帧推进机制） | 同上 |
| 视频/音频 | **零能力**：`readyState` 恒 0、`duration=NaN`、`currentTime` 不推进 | [media-format-verification-plan.md](media-format-verification-plan.md) §3.3 D5 |
| ModeToolkit 光栅图 | **一律不渲染**（连 `data:` URI 也不渲染），需宿主显式 `AllowHostResolved` | 同上 §0 第 5 条 |

### 1.2 调试通道（主线 B 的起点）

**不存在 CDP**。全仓 Go 源码搜 `cdp|chromedp|9222|json/version|Runtime.evaluate|remote-debugging`
只命中 2 处注释（`engine/style/scrollbar.go:6`、`engine/rendering/scrollbar_classic_geometry_test.go:25`，
内容都是「用 Playwright 连 Edge 的 9222 端口量滚动条几何」**方法记录**，不是实现）；
`go.mod` 无任何 WebSocket 库；全仓 `net.Listen|ListenAndServe` 仅 1 处
（`dev/probes/gouide_real_e2e/main.go:296`，探针临时 HTTP）。

现有替代能力（**供 CDP 适配层复用**）：

| 现有能力 | 位置 |
|---|---|
| `EvalJS(script)` / `CallFunction(name, args...)` | `webkit/webview.go:1561` / `:1604` |
| `Render() ([]byte)`（截图）/ `RenderHTML(id, html)` / `RenderView()` | `webkit/webview.go:1499` / `:1654` / `:2419` |
| `Document()` / `ConsoleOutput()` / `JSInterpreter()` | `webkit/webview.go:1724` / `:1676` / `:1719` |
| `LoadURL` / `LoadHTMLWithBaseURL` / `LoadHTML` | `webkit/webview.go:1317` / `:819` / `:805` |
| 鼠标：`Interaction.MouseButton/MouseMove/Wheel/MouseLeave` | `webkit/interact.go:163` / `:306` / `:479` / `:354` |
| 键盘：`FormFocus.CharInput(rune)` / `KeyInput(name)` / `CtrlShortcut(name)` | `webkit/formfocus.go:451` / `:484` / `:767` |
| 页面内调试对象 `window.__devtools`（`tree/pick/sel/page`） | 定义 `app/host.go:46-54`，注入 `:421`，`WB_DT_DUMP=1` 帧 dump `:3242` |
| 无头自检 / 布局审计 | `cmd/psai -verify` / `-audit`（`cmd/psai/main.go:38-42`） |

> ★ **`__devtools` 与 CDP 的关系**：前者是**进程内、JS 注入式**的观测对象（只有宿主能问），
> 后者是**跨进程协议**。CDP 的 `Runtime.evaluate` 适配可直接复用 `EvalJS`；
> `DOM.*` 可实现为对 `__devtools` 同一份数据源的协议化封装——两者**共用数据源、不互斥**。

### 1.3 JS 引擎（主线 C 的起点）

**架构**（`engine/js/`，4 个子包）：

```
engine/js/goja/      119 个 .go —— vendored 上游 goja（含本仓库补丁）
engine/js/jsc/      4235 行   —— 引擎适配层（包名 JSC，实现是 goja）：
                                 Interpreter / JSValue / JSObject / JSFunction /
                                 EventLoop / streams / webapi / perfapi / lazy
engine/js/bindings/   60 个 .go —— DOM/Web API 绑定层（依赖 jsc 抽象）
engine/js/worker/     2 个 .go —— 每个 Worker 独立 goja runtime + goroutine
```

**耦合面（换引擎成本的量化依据）**：

| 项 | 数值 | 含义 |
|---|---|---|
| `bindings` 中直接引用 `goja.*` 的文件 | **4 个**（真正耦合 1 个：`dom.go:4675-4955` 的 `styleProxy`，11 处） | bindings 基本与引擎无耦合 |
| 使用 `jsc.Interpreter` 的文件 | 45 个 | 走抽象层 |
| `jsc.JSValue` 定义 | `struct { v goja.Value; nativeFn NativeFunc; interp *Interpreter }` | ★ **不是 interface** → 换引擎必须重写 jsc 实现，但**方法签名可保持不变** |
| 上游补丁（升级时必须保留） | `strFlags`（`goja/object_dynamic.go`）、`symValues` | 见 `docs/TECH_DEBT.md`「vendored goja」节 |

**性能实测（本轮，本机 Windows / node v22.17.0 对照；复现命令见附录）**：

| 基准（`dev/output/jsbench.js`） | goja | node 22 (V8) | 差距 |
|---|---|---|---|
| fib(27) 递归 | 90 ms | 13 ms | 6.9× |
| array 100k push+sort+sum | 642 ms | 53 ms | 12.1× |
| **string 50k concat** | **2320 ms** | 10 ms | **232×** |
| **object prop 1M rw** | **519 ms** | 6 ms | **87×** |
| json roundtrip 20k | 283 ms | 39 ms | 7.3× |
| regex 20k test | 23 ms | 3 ms | 7.7× |
| closure 500k calls | 181 ms | 4 ms | 45× |
| try/catch 200k | 88 ms | 270 ms | **goja 快 3.1×**（V8 侧不擅长的路径） |

> 与 2026-09-27 的独立实测一致（`scripts/v8/README.md` §5：91/690/2700/590 ms）⇒ 数据可复现。
> **读法**：纯计算差距 **7×–232×**；但 V8 的**跨界**（JS↔Go 每次调用）反而慢
> **7.4×–240×**（同文件 §5 表：`obj.Set` 67×、`obj.Get` 240×、JS→Go 回调 7.4×）。
> ⇒ 换 V8 的收益取决于代码「在 JS 里跑」还是「在边界上跑」——这是主线 C 的核心杠杆。

**ES 特性缺口（权威清单：`engine/js/goja/tc39_test.go` 的 `featuresBlackList`，按类归并）**：

| 类 | 缺失项 |
|---|---|
| ES 模块 | `dynamic-import`、`import.meta`、`import-assertions/attributes`、`source-phase-imports`、`import-defer` ⇒ **完全没有模块系统** |
| 异步迭代 | `async-iteration`、`Symbol.asyncIterator` |
| ArrayBuffer 系 | `resizable-arraybuffer`、`immutable-arraybuffer`、`arraybuffer-transfer`、`uint8array-base64`、`Float16Array` |
| 正则 | `v-flag`、`match-indices`(d 标志)、`unicode-property-escapes`、`duplicate-named-groups`、`modifiers`、`RegExp.escape`、`legacy-regexp` |
| 并发/内存 | `Atomics`(+`waitAsync`/`pause`)、`SharedArrayBuffer`、`WeakRef`、`FinalizationRegistry`、`symbols-as-weakmap-keys` |
| 新标准 | `Temporal`、`ShadowRealm`、`decorators`、`explicit-resource-management`(using)、`set-methods`、`iterator-helpers`、`joint-iteration`、`iterator-sequencing`、`promise-try`、`promise-with-resolvers`、`array-grouping`、`Array.fromAsync`、`Math.sumPrecise`、`String.prototype.toWellFormed/isWellFormed` |
| 其他 | `tail-call-optimization`（规范已弃用，可忽略） |

> 注：本仓库已在 `jsc/webapi.go` **自行补齐**了部分 Web 侧能力（`structuredClone`（`webapi.go:396`）、
> `WeakRef`/`FinalizationRegistry` 降级实现（`webapi.go:418`））——说明「缺什么我们补什么」是既有做法，
> 但按此方式补齐 `Temporal`/模块系统/`Atomics` 的成本远超换引擎。

**已有 V8 接入实测（2026-09-27，[scripts/v8/README.md](../scripts/v8/README.md)）**：MSYS2 `mingw-w64-x86_64-v8 11.9` + 本地 fork 的
v8go（补丁 6 行）在 Windows 上**编译/链接/运行/基准全通过**；已知坑：跨平台版本不一致（Windows 11.9 /
Linux-macOS 11.1）、`NewValue(int64)` 造 BigInt、分发 +60MB（libv8.dll 28.7MB + ICU 等）。

---

## 2. 主线 A：媒体真实播放（视频 / 音频）

**归属**：实现属本仓库（宿主注入层 + 引擎）；能力等级**验证**属独立媒体验证项目
（`docs/media-format-verification-plan.md`）。

| 阶段 | 内容 | 验收（对应该文档） |
|---|---|---|
| **A0** 元数据（前置，成本最低） | 宿主用 `ffmpeg -i` 探测时长/尺寸 → 注入 `MediaMetadataResolver`（决策 4 的 U8） | `TC-M-501/601` 的 **L1**：`readyState ≥ 1`、`duration≈1s`、`loadedmetadata` 派发 |
| **A1** 视频出画面（L2） | ✅ **已实装**（2026-10-07，路线 1，见 §0.1 与下方实现形态） | `TC-M-502` 截图有画面（非灰块）；`poster` 正确绘制——两者都有实测判据（A1-1/2/3） |
| **A2** 视频动态（L4） | ✅ **已实装**（2026-10-07）：`timeupdate` 驱动的换帧重绘、**异步预取 + 帧队列**、**精确到帧的 seek**（帧对齐规则）、**帧率驱动的预取窗口**、**`requestVideoFrameCallback` + 帧就绪重绘**、**连续帧采样** | `TC-M-503/504/507`：事件序列（A2-3）、`currentTime` 跳转后画面（A1-3/A2-5：20 个采样点都落在该时刻所属帧）、播放中画面随时间变化（A2-1/A2-6：连续 5 帧都等于各自参照）、帧呈现回调（A2-7）、播放全程同步抽帧 0 次（A2-4） |
| **A3** 音频（L1→L4-S） | ✅ **已实装**（2026-10，按 [`audio-backend-proposal.md`](audio-backend-proposal.md) 的「宿主注入 PCM + 输出后端分阶段」）：宿主 `app/mediaaudio.go` 用 ffmpeg 解 **s16le PCM** 注入引擎（`engine/rendering/audioframe.go` 的注入通道），`engine/js/bindings/mediaaudio.go` 的会话把它推给输出后端并**以输出位置为主时钟**驱动 `currentTime`；输出后端 `app/audioout_windows.go`（waveOut，位置查询实测用 `TIME_BYTES=0x0004`）与 `app/audioout_other.go`（非 Windows 降级为挂钟推算——**播放不停摆**，只是时钟精度降级） | `TC-M-601/602` **L1→L4**：判据 A（PCM **49041 帧**、主峰 **439.88Hz**（期望 440Hz）、幅度 1.000）＋判据 B（环回录音比对；无设备时记 `SKIP(no-loopback)`，有候选但未路由时记 `mismatch`）＋ TC-M-602 定点（`currentTime` 随输出位置推进、终态到 1.00s）。`TC-M-603`（WebAudio）仍缺，见 `media-format-verification-plan.md` §9.6 |
| **A4** 动图（W3C 之外的自家能力） | ✅ **已实装**（2026-10-07）：goskia 暴露 `SkCodec` 多帧（`skia.NewCodec` / `DecodeFrames` / `FrameDurationMS`），引擎侧 `engine/rendering/imageanimation.go` 按帧时长选帧（宿主注入帧序列，落点见 §0.1 的 A4 行） | **连续帧差异**：判据 A4-1（6 次采样出现 3 种帧色且都等于样本帧色） |

**A1 的两条实现路线（立项时定，决策 4 已采纳「立项时定」）**：

1. **宿主注入帧流**（最小侵入）—— ✅ **已采纳并实装**：宿主自带解码（本地 ffmpeg），把**帧位图字节**
   交给引擎；引擎只管画。**优点**：引擎不背解码器体积与许可；**缺点**：宿主必须自带 ffmpeg。
   实现形态（与设计稿的差异）：不是「推 RGBA 帧流」，而是**按需取帧**——绑定层只声明
   `(url, 时间点, show poster flag, Playing)`，painter 在绘制时按 `(url, 时间点)` 要一帧
   （PNG 字节），渲染层按同一键缓存已解码帧。好处是宿主天然按需抽帧（不播放就不抽）、缓存键与
   显示状态一一对应。取帧分两条路（A2 已实装）：静止态**同步**要（`VideoFrameSource`——首帧 /
   seek / 暂停必须在这**一次**绘制里画出结果），播放推进**异步**要（`VideoAsyncFrameSource` +
   绑定层预取下一时刻，渲染线程不等解码，未交付先显示上一帧）。此前「一律同步」意味着每换一帧
   渲染线程都要跑一次 ffmpeg（见 TECH_DEBT 的历史条目）。
2. **引擎内置 ffmpeg**（能力最全）：体积/许可/跨平台成本高，需单独审核 —— **未采纳**。

**依赖**：goskia 暴露 `SkCodec`（跨仓库联动见 §5）——A1 **不依赖**此项（帧由宿主抽成 PNG，
走引擎既有的位图解码路径）；它只挡 A4（动图多帧）。

---

## 3. 主线 B：CDP 调试协议服务端（新增）

### 3.1 目标与典型场景（按优先级）

| 优先级 | 场景 | 需要的域 |
|---|---|---|
| **S1** | **wb-ui 自家测试工装**用统一协议驱动引擎（替代 `-verify`/`-audit` 里的一次性 API 调用） | `Runtime` + `Page.captureScreenshot` + `Input` + `DOM`(读) |
| **S2** | **Chrome DevTools 前端**连上引擎看 Elements / Console | `DOM` + `CSS` + `Runtime` + `Log` + `Page` |
| **S3** | 外部自动化框架（Puppeteer 子集）驱动 | 追加 `Target`(扁平会话) + `Page.navigate` + `Emulation` |
| **不承诺** | **Playwright `connect_over_cdp`** | 需要 `Network`/`Page`/`Runtime` 全套 + 会话语义细节，成本极高，**明确不做** |

### 3.2 架构（4 层）

```
HTTP 端点                       WS 传输                 协议分派                引擎适配
GET /json/version   ─┐
GET /json/list       ├─→  net/http (127.0.0.1)  ─→  WebSocket  ─→  JSON-RPC  ─→  cdp.Adapter
PUT /json/new        │      （仅 loopback）          (RFC6455 子集)   分派 + 会话          │
GET /json/protocol  ─┘                                                          ↓
                                                       Target/Session 模型     WebView / __devtools
                                                       （flatten 模式）          （webkit/*.go）
```

| 组件 | 新包 | 职责 |
|---|---|---|
| WS 传输 | `engine/devtools/cdp/ws`（或第三方库） | RFC6455 握手 + 帧编解码（掩码文本帧 / ping / pong / close / 分片；**不做** permessage-deflate） |
| 协议分派 | `engine/devtools/cdp` | 消息路由（`id`/`method`/`params`、`sessionId`）、错误码、事件推送 |
| Target/Session | 同上 | 单页面 Target（`targetId` = 页面实例）；可选 **Worker target**（`engine/js/worker` 的独立 runtime 天然可作 Target）；`Target.attachToTarget {flatten:true}` → `sessionId` |
| 域实现 | `engine/devtools/cdp/domains` | `Runtime` / `Page` / `DOM` / `CSS` / `Input` / `Log` / `Emulation` / `Browser` |
| 引擎适配 | `engine/devtools/cdp/adapter.go` | 把域方法映射到 `webkit.WebView` / `__devtools`（映射表见 3.3） |

### 3.3 域方法 → 现有能力映射（全部有据可依）

| CDP 方法 | 映射到 | 位置 | 备注 |
|---|---|---|---|
| `Browser.getVersion` | 静态字串（产品名/协议版本/引擎版本/UA） | — | S1 必需（客户端握手） |
| `Target.getTargets` / `attachToTarget` / `detachFromTarget` | Target 注册表（新） | — | `flatten:true` 必做 |
| `Runtime.evaluate` | `WebView.EvalJS` | `webkit/webview.go:1561` | `returnByValue` 走 `jsc.JSValue` → RemoteObject |
| `Runtime.callFunctionOn` | `WebView.CallFunction` | `:1604` | 需维护 `objectId` 表（JSValue 句柄） |
| `Runtime.getProperties` | `jsc.JSObject` 反射遍历 | `jsc/goja_adapter.go` | 返回 `PropertyDescriptor` 列表 |
| `Runtime.consoleAPICalled`（事件） | `WebView.ConsoleOutput` / `jsc.BufferLogger` | `:1676` | 需把缓冲日志改为**增量**回调（现为一次性字符串） |
| `Runtime.exceptionThrown`（事件） | 引擎异常钩子（`EvalJS` error / `onerror`） | `:1561` | P1 |
| `Page.captureScreenshot` | `WebView.Render()` | `:1499` | 返回 PNG 字节 → base64 |
| `Page.navigate` / `reload` | `WebView.LoadURL` / 重新加载 | `:1317` | ★ ModeToolkit 下受**资源策略**限制（`DenyExternal` 时外部资源全拦） |
| `Page.enable` + `Page.loadEventFired` | 帧/文档加载回调 | `webkit/webview.go:1411` | P1 |
| `DOM.getDocument` / `querySelector` | `WebView.Document()` + `__devtools.tree/sel` | `:1724` / `app/host.go:46-54` | 节点 id ↔ `dom.Node` 映射表 |
| `DOM.getBoxModel` | layout box 几何（`RenderView` / `ElementBox`） | `webkit/webview.go:2419` | S2 必需（Elements 面板高亮） |
| `CSS.getComputedStyleForNode` | 样式解析结果（`engine/style`） | — | S2 必需 |
| `Log.entryAdded`（事件） | 同 Console 管道 | — | S2 |
| `Input.dispatchMouseEvent` | `Interaction.MouseButton/MouseMove` | `webkit/interact.go:163` / `:306` | 需坐标变换（CDP 用 CSS 像素，与引擎一致 ✓） |
| `Input.dispatchMouseEvent{type:mouseWheel}` | `Interaction.Wheel` | `:479` | — |
| `Input.dispatchKeyEvent` | `FormFocus.CharInput`（文本）/ `KeyInput`（命名键） | `webkit/formfocus.go:451` / `:484` | `windowsVirtualKeyCode`→键名映射已有：`app/host.go:4553-4688` |
| `Emulation.setDeviceMetricsOverride` | `WebView.Resize`（宽高） | `:1538` | ★ **无 deviceScaleFactor/zoom 支持** → 该字段标注「部分实现」或先做引擎侧 DSF |

### 3.4 传输实现选型（需决策）

| 方案 | 成本 | 风险 |
|---|---|---|
| **(a) 自研最小 WS**（推荐） | ~300–400 行 + 测试，**零新依赖**（与本仓库 `go.mod` 仅 8 个直接依赖的风格一致） | 握手/帧细节需自测（可用 Edge 当客户端对照） |
| (b) 引入 `gorilla/websocket` | 1 个新依赖（BSD-3） | 依赖面增加；成熟稳定 |

> 两者都把传输藏在内部包接口后面，**可后续互换**。选 (a) 的理由：CDP 客户端只需
> 「掩码文本帧 + ping/pong/close」，子集极小；且必须自己写的是**分派与会话**，不是帧层。

### 3.5 接线点（宿主开关）

| 宿主 | 接线 |
|---|---|
| `app/host.go`（GLFW 主机） | 新增 `--remote-debugging-port=N`；`N>0` 时启动服务端，**仅绑 `127.0.0.1`** |
| `cmd/psai/main.go` | 同上（无头自检时也能被外部工具驱动） |
| 默认 | **端口 0 = 关闭**（零开销，不监听）；开启时 stderr 打印一行 `DevTools listening on ws://…`（与 Chrome 同格式） |

### 3.6 验收判据（可执行）

1. `curl http://127.0.0.1:9222/json/version` → 含 `Browser`/`webSocketDebuggerUrl` 字段；`/json/list` 含页面 target。
2. WS 上 `{"id":1,"method":"Runtime.evaluate","params":{"expression":"1+1","returnByValue":true}}`
   → `result.value == 2`。
3. `Runtime.evaluate {expression:"document.title"}` 与 `wv.EvalJS("document.title")` 结果一致。
4. `Page.captureScreenshot` 解码后的 PNG 与 `wv.Render()` **逐像素一致**（用 `dev/tools/pixdiff.py`）。
5. `Input.dispatchMouseEvent` 派发后，`__devtools.pick(x,y)` 与注入前观测到的命中共对象一致。
6. `DOM.getDocument` 的节点数与 `__devtools.tree(0)` 计数一致。
7. 目标端口**默认关闭**；开启时只监听 loopback（`netstat -ano | findstr 9222` 验证）。

**S2（DevTools Elements 所需域）判据**（自动化在 `cmd/psai/devtools_selftest.go`，随 `-verify` 一起跑）：

8. **Runtime 句柄表**：`Runtime.evaluate {expression:"({a:1,b:'两'})"}`（缺省 `returnByValue=false`）
   → 结果含 `objectId`；`Runtime.getProperties` 列出 `a`/`b`；`Runtime.callFunctionOn
   {objectId, functionDeclaration:"function(){return this.b;}"}` = `"两"`；`Runtime.releaseObject` 成功。
9. **DOM 扩展**：`DOM.querySelectorAll {selector:"div"}` → 非空 nodeIds；首节点 `DOM.getAttributes`
   非空、`DOM.getOuterHTML` 以 `<div` 开头、`DOM.resolveNode` → `objectId`。
10. **DOM 编辑**：`DOM.setAttributeValue` 后页面 `getAttribute` 读到该值；`DOM.removeAttribute` 后不再读到。
11. **CSS 域**：对注入的探针元素（内联样式 + 一条 `<style>` 规则）分别取 computed（非空）、
    inline（非 null）、matched（含注入规则）。
12. **console 分级**：`console.error/warn` → `Log.entryAdded` 的 `entry.level` 为 `error`/`warn`
    （判据先推平 `enable` 后的历史存量，再按**探针文本**等事件——增量通道的第一批是旧日志）。

---

## 4. 主线 C：JS 引擎 JIT 与完整 JS 对标

### 4.1 判定（先说清不可能的事）

| 诉求 | goja 现状 | 结论 |
|---|---|---|
| **JIT** | 无；上游定位「不是 V8/SpiderMonkey 的替代品」（[engine/js/goja/README.md](../engine/js/goja/README.md) FAQ） | **改造 goja 得不到 JIT**——等于自研引擎 |
| **完整 ES 对标** | 40+ 特性缺失（§1.3 清单） | 只能逐项自研；且 `Temporal`/模块系统/`Atomics` 等**成本极高** |
| **性能** | 纯计算慢 7–232× | 靠「热点优化」可期望改善，但**不可能追平 JIT** |

### 4.2 三条路线对比

| 维度 | **C1 深化 goja** | **C2 V8 后端**（v8go fork） | **C3 QuickJS** |
|---|---|---|---|
| JIT | ❌ 无 | ✅ 有（TurboFan/Maglev） | ❌ 无（但解释器比 goja 快数倍） |
| ES 覆盖 | ES2015+ 大部分，**无模块** | ✅ 最新标准（含 `Intl`、`Temporal` 实验） | ✅ ES2020+（含模块） |
| 纯计算速度 | 1×（基准） | **7–232×** | ~2–5×（估） |
| 跨界成本 | **最快**（进程内、同堆） | 慢 7.4–240×（Locker + Scope + 装箱） | 中等（cgo，需设计句柄表） |
| 分发体积 | 0 | **+60MB**（libv8.dll 28.7MB + ICU） | +~1MB |
| 构建复杂度 | 0（纯 Go） | 高（MSYS2 SDK 87MB；Win 11.9 / Linux-macOS 11.1 版本不一致） | 中（cgo，静态库可 vendored） |
| 许可 | MIT | BSD-3 | MIT |
| 本仓库现状 | Go 官方路径；有本地补丁需维护 | **已实测跑通**（[scripts/v8](../scripts/v8/README.md)） | 未评估 |
| 风险 | 天花板明确（永远无 JIT） | 跨界回归、体积、跨平台不一致 | 需新写整套绑定层，工作量大且无 JIT |

### 4.3 推荐路线：**后端化 + 双后端**（不是二选一）

```
              ┌──────────────────────────────┐
bindings ───→ │  engine/js/jsc (适配层 API)   │  ← 签名不变（Interpreter/JSValue/...）
  (60 文件)   └───────────┬──────────────────┘
                          ↓  后端接口（新增）
             ┌────────────┴────────────┐
        goja 后端（默认，纯 Go）     V8 后端（可选，cgo）
        engine/js/goja/*           scripts/v8 + v8go fork
```

**理由（全部由 §1.3 的量化事实支撑）**：

1. `jsc` 已经是**唯一**的适配面（45 个文件用它、bindings 只 4 个文件直连 goja）
   ⇒ 后端化的改动面**已知且可控**。
2. V8 的收益与代价都是**可量化**的：纯 JS 快 7–232×、跨界慢 7.4–240×
   ⇒ 胜负取决于「热点在 JS 内还是边界上」，必须**先测后选**（而不是全局切换）。
3. 双后端让「完整 ES 对标」变成**可选能力**：需要 `Temporal`/模块/`Intl` 的场景用 V8 后端，
   其余场景保持纯 Go 零依赖（分发体积不涨）。

### 4.4 关键杠杆：减少 JS↔Go 跨界（先于换引擎）

V8 的跨界成本表（[scripts/v8/README.md](../scripts/v8/README.md) §5）说明：**每次跨界 1–5 µs**，每帧 1 万次 ≈ 15 ms。
因此 V8 后端上线前必须先做：

1. **批量属性读写**：`style` 多属性一次设置（现在可能逐条跨界）；
2. **事件派发聚合**：一帧内多次派发合并到一次跨界；
3. **绑定层句柄缓存**：`objectId`/包装对象复用，避免每次调用重建 Persistent。

> 与本仓库既有结论一致：[docs/TECH_DEBT.md](TECH_DEBT.md)「goja 引擎热点…**真实热点必须先 `goja.StartProfile` 采样定位**」
> ——本主线把该原则升级为**切换后端前的前置门禁**。

### 4.5 分期

| 阶段 | 内容 | 交付物 / 验收 |
|---|---|---|
| **C-P0** 基线固化 | ① 把 `dev/probes/jsbench` 做成**按需工装**（本机 node 对照）；② 新增 `dev/probes/jsesmatrix`：以 `featuresBlackList` 为清单逐项探测当前**支持/缺失/降级**，输出 JSON 快照 | 两份基线：性能（§1.3 表）与特性矩阵（可回归比对） |
| **C-P1** goja 深化（低风险先做） | ① 跟进上游版本、**保留 `strFlags`/`symValues` 补丁**（`docs/TECH_DEBT.md` 有完整保留清单）；② 按业务驱动补特性（优先级：`async-iteration` → `Array.fromAsync`/`iterator-helpers` → `v-flag`/`d-flag`）；③ 用 `goja.StartProfile` 定位真实热点后再优化 | ① 特性矩阵新增项转 ✅；② 性能基线**不退化**（jsbench 逐项 ≤1.1× 现状） |
| **C-P2** 后端接口抽象 ✅ **已实装**（2026-10-07） | goja 的类型与构造器全部收敛到 `engine/js/jsc/backend_goja.go`（**全包唯一** import goja 的文件，对外只用 `be*` 名）；契约 `Backend` / `RuntimeHandle` + 后端注册表在 `backend.go`；`bindings` **零改动**（它只用 jsc 的公开 API） | 见 §0.1 的 C-P2 行：jsc 里 goja import 只剩后端文件、`bindings` 无 `be*` 引用、构建与测试绿、28 项自检不变。**真正的 V8 后端**（`backend_v8.go` + SDK 接入）属 C-P3 |
| **C-P3** V8 后端（可选） | ① `scripts/v8` 的 SDK 获取 + v8go fork **入库**（含补丁与构建脚本）；② 实现 `backend-v8`；③ worker 也需支持（每 worker 一个 Isolate） | ① jsbench 8 项 vs node 差距 **≤3×**；② 一致性套件（`dev/suites/*`）在双后端下**逐项一致**；③ 分发策略落地（+60MB 的体积/许可/CI 说明） |
| **C-P4** 跨界优化（与 P3 并行） | §4.4 三项 | 跨界微基准（`dev/probes/jsboundary`）改善；真实应用交互指标改善 |

### 4.6 验收指标（诚实版）

1. **特性**：`jsesmatrix` 的「浏览器对标」列中，`featuresBlackList` 各项必须落到
   `支持 / 明确豁免 / 已排期` 三态之一（**不允许「未知」**）。
2. **性能**：V8 后端 jsbench vs node **≤3×**；goja 后端**不退化**。
3. **真实应用**：[docs/PERF_BASELINE.md](PERF_BASELINE.md) §5 的编辑器指标（滚动 3.56 s/op、输入 104 ms/op）。
   ★ **重要边界**：该文档已用 A/B 对照实验证明**滚动成本的 71% 在引擎 `scrollTop` 写入路径**、
   `readLayoutOnly = 0 ms` ⇒ **换 JS 引擎不会解决滚动问题**（那是布局/事件/失效路径的活）。
   JS 主线的真实应用收益主要落在**输入路径**（104 ms）与 Vue 组件渲染/更新（模板编译、响应式副作用），
   必须先用 `goja.StartProfile` 拆出 JS 占比，再谈收益倍数。

---

## 5. 依赖与跨仓库联动

| 主线 | 是否动 goskia | 联动要点 |
|---|---|---|
| A 媒体 | ✅（`SkCodec` 暴露 + 音频后端可能） | **完整链路已实走一次（2026-10-07，A4）**：goskia 提交 `skia/codec.go` → `git push origin main`（`5015494a`）→ wb-ui `GOWORK=off GOSUMDB=off GOPROXY=direct go get github.com/hoonfeng/goskia@5015494a`（模块 zip 较大，约 10 分钟；直连比 goproxy.cn 快）→ `GOWORK=off go build ./...` + 测试 → `go.mod` 落到 `v0.0.0-20261006194810-5015494aa077` |
| B CDP | ❌ 纯 wb-ui（Go 侧自建 WS） | 无 |
| C JS 引擎 | ❌（goja 已并入本仓库 `engine/js/goja`） | 若走 V8：新增 `scripts/v8` 构建链 + cgo/CGO 开关；CI/分发需注明；**与本仓库默认纯 Go 构建路径并存**（V8 后端必须可编译隔离，不能破坏 `GOWORK=off go build ./...`） |

---

## 6. 统一分期总表

| 期 | 主线 A 媒体 | 主线 B CDP | 主线 C JS 引擎 | 门槛 |
|---|---|---|---|---|
| **P0** 底盘 | A0 宿主注入元数据（U8） | B0 自研最小 WS + `/json/version`+`/json/list` + `Runtime.evaluate` | C-P0 基线固化（jsbench 工装 + jsesmatrix 探针） | 全部为低风险、可独立验收 |
| **P1** 可用 | A1 视频出画面（L2） | B1 `Page.captureScreenshot` + `Input.*` + `DOM`(读) → **S1 场景可用** | C-P1 goja 深化（上游跟进 + 补丁保留 + 特性补齐 + profile 定位热点） | 每项都要有可执行验收（§3.6 / §4.5） |
| **P2** 对标 | A2 视频动态（L4）+ A4 动图（`SkCodec`） | B2 `DOM`+`CSS`+`Log` 完整 → **S2 DevTools Elements** | C-P2 后端接口抽象（bindings 零改动） | 需要设计评审 |
| **P3** 代价最高 | A3 音频后端 | B3 `Target` 扁平会话 + `Emulation` + `Page.navigate` → S3 | C-P3 V8 后端 + C-P4 跨界优化 | **必须用户确认**（体积/许可/构建复杂度） |

---

## 7. 风险与不做项

| 项 | 结论 |
|---|---|
| 在 goja 内实现 JIT | **不做**（等价自研引擎；上游无此路线） |
| Playwright `connect_over_cdp` 兼容 | **不承诺**（域覆盖面 × 会话语义，成本远超收益） |
| 用 `__devtools` 替代 CDP | **不做替代，做共存**：CDP 的 `DOM`/`Runtime` 适配层复用同一数据源 |
| 引擎内 `WebSocket` 接真实网络以承载 CDP | **不做**：CDP 服务端在 Go 侧（引擎 WS 是宿主注入 stub，见 `docs/TECH_DEBT.md`） |
| V8 后端作为**默认**引擎 | **不做**：默认保持纯 Go（零依赖/小体积），V8 仅作可选后端 |
| 升级 goja 上游时丢弃本地补丁 | **禁止**：`strFlags` 等是 Vue 3.5 挂载的必要条件（`docs/TECH_DEBT.md`） |

---

## 8. 待决策点（需用户选定后才进入实施）

| # | 决策项 | 选项 | 影响 |
|---|---|---|---|
| 1 | **CDP 目标场景** | (a) 仅 S1 自家工装 /(b) S1+S2 DevTools 前端 /(c) 追加 S3 自动化框架 | 决定域子集规模（P0 单点 vs P2 完整） |
| 2 | **CDP 传输实现** | (a) 自研最小 WS（零依赖）/(b) `gorilla/websocket` | 依赖面 vs 实现风险 |
| 3 | **JS 引擎路线** | (a) 仅 C1 深化 /(b) C1+后端化（推荐）/(c) 直接 V8 单后端 /(d) 评估 QuickJS | 决定 §4.5 是否做 P2/P3 |
| 4 | **分发体积约束** | 能否接受 **+60MB**（V8） | 若不可接受，C2/P3 直接砍掉，主线 C 只剩 C1（则「JIT」诉求**无法满足**，需显式接受） |
| 5 | **实施顺序** | 三线并行 / 先 B（成本最低、立竿见影）/ 先 A（业务可见） | 资源分配 |

---

## 附录 A：证据索引

| 主题 | 证据 |
|---|---|
| CDP 不支持 | 全仓搜 `cdp\|chromedp\|9222\|json/version\|remote-debugging` = 2 处注释；`net.Listen` 仅 `dev/probes/gouide_real_e2e/main.go:296`；`go.mod` 无 WS 库 |
| 现有调试/驱动能力 | `app/host.go:46-54`（`__devtools` 定义）、`:421`（注入）、`webkit/webview.go:805/819/1317/1411/1499/1538/1561/1604/1654/1676/1719/1724/2419`、`webkit/interact.go:163/306/479/354`、`webkit/formfocus.go:451/484/767`、`cmd/psai/main.go:38-42` |
| JS 架构与耦合面 | `engine/js/jsc/goja_adapter.go:374`（`JSValue` 定义）、`engine/js/bindings/dom.go:4675-4955`（styleProxy）、`engine/js/worker/worker.go`（每 worker 独立 runtime） |
| goja 特性缺口 | `engine/js/goja/tc39_test.go`（`featuresBlackList`） |
| goja 性能 | 本轮实测（附录 B）+ `scripts/v8/README.md` §5 |
| V8 接入实测 | `scripts/v8/README.md`（全文）+ `scripts/v8/v8go-win.patch` |
| goja 本地补丁 | `docs/TECH_DEBT.md`「vendored goja：动态对象描述符语义放宽（strFlags）」 |
| 媒体现状 | `docs/media-format-verification-plan.md` §0/§3 |
| 性能真实边界 | `docs/PERF_BASELINE.md` §5（滚动 71% 在引擎侧，非 JS） |

## 附录 B：复现命令

```bash
# goja 性能基线（纯 Go，无需 CGO）
GOWORK=off go run ./dev/probes/jsbench dev/output/jsbench.js
# V8 对照（本机 node 22）
node dev/output/jsbench.js

# 跨界微基准（goja 侧）
go run ./dev/probes/jsboundary

# JS 特性矩阵（C-P0 基线；-v 逐项打印，缺探测定义会报错退出）
go run ./dev/probes/jsesmatrix -v -out dev/output/jsesmatrix.json

# 端到端自检（B0/B1 判据 1-7 + 媒体 A0 的 L1 断言 + A1/A2 判据）
CGO_ENABLED=1 go build -o _temp/psai.exe ./cmd/psai
./_temp/psai.exe -verify -png "" -remote-debugging-port 9355   # 29 项 ✓ / 0 项 ✗
#   A1/A2 判据样本由自检自己生成到 _temp/mediaverify/（不手写期望像素：
#   参照值来自独立 ffmpeg 抽帧，见 cmd/psai/mediaframe_selftest.go；
#   帧精度类判据的参照走**按帧号**的 -vf select=eq(n,k)，与抽帧用的 -ss 是两条路）
#   判据：A1-1/2/3（出画面 / poster 优先 / seek 让位）+ A2-1/2/3（帧推进 / 末尾帧 / 事件序列）
#         + A2-4（播放全程同步抽帧 0 次：帧靠绑定层预取 + 宿主异步交付）
#         + A2-5（精确到帧的 seek：20 个采样点 = 该时刻所属帧）
#         + A2-6（连续帧采样 TC-M-507：连采 5 帧、每帧 = 该帧参照）
#         + A2-7（requestVideoFrameCallback：回调 4 次、presentedFrames 1..4 递增）
#   CDP 判据 1-7（B0/B1：端点 / evaluate / 截图逐像素 / 输入 / 域节点数 / 仅 loopback）
#         + 判据 8-12（B2/S2：Runtime objectId 句柄表、DOM 扩展与编辑、CSS 三个方法、
#           console 分级 → Log.entryAdded）
#   A4-1（动图：6 次采样出现 3 种帧色，每帧 300ms——探针用 data: URI 注入，
#      且排在 CDP 判据之后：它要采样约 2 秒，先跑会让 CDP 判据的目标元素不在初始状态）

# 外部视角复核（另开终端：只监听 loopback + 非 Go 客户端能驱动）
netstat -ano | findstr 9355
python _temp/cdp_probe.py 9355

# 媒体解码矩阵（设计阶段探针，产物在 _temp/，不入库）
go run ./_temp/mediacheck/main.go

# C-P2 后端接口抽象的验收（三条都可直接跑）
grep -l 'engine/js/goja"' engine/js/jsc/*.go      # 只应有 backend_goja.go（+ 测试）
grep -rn 'beValue\|beObject' engine/js/bindings/  # 应为空：bindings 不用后端名（零改动）
GOWORK=off go build ./...                          # 换后端前的完整构建检查
```
