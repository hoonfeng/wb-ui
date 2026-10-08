# A3 音频后端立项评估（2026-10）

> 归属：主线 A（媒体真实播放）P3 阶段项，对应
> [`implementation-path.md`](implementation-path.md) §2 的 **A3 音频（L1→L4-S）** 与
> [`media-format-verification-plan.md`](media-format-verification-plan.md) §5 **G6**（TC-M-601..604）。
> 本文档是**立项材料**，不是实施记录；实施需用户确认后再动（§8 决策点 4「必须用户确认」）。

> **实施状态（2026-10-08，A3-3 完整子集）**：WebAudio 已**从最小子集扩展为完整子集**
> （决策点 3 已确认「要」）。两部分都不回退：
>
> **① 最小子集（原样保留，TC-M-603 的既有取证继续成立）**：`AudioContext`（构造 +
> `sampleRate` / `state` / `currentTime` / `destination` / `close`）、`decodeAudioData`
> （**真解码**：宿主 ffmpeg → `AudioBuffer`，未注入解码器时如实 reject）、`AudioBuffer`
> （`length` / `duration` / `sampleRate` / `numberOfChannels` / `getChannelData`）、
> `Float32Array` 量纲。判据实测见 `dev/media/out/report.md` TC-M-603 小节
> （44100Hz / 1ch / 44100 帧 / 1.000s，判据 A 同一套 FFT：主峰 439.95 Hz）。
>
> **② 完整子集（本次新增：AudioNode 图 + AudioParam 自动化 + OfflineAudioContext + 实时输出）**：
>
> - **节点**：`GainNode`、`OscillatorNode`（sine / square / sawtooth / triangle，相位按每帧推进
>   以支持频率扫描）、`AudioBufferSourceNode`（loop / loopStart / loopEnd / playbackRate /
>   detune，线性插值）、`ConstantSourceNode`、`StereoPannerNode`（等功率定律）、
>   `DelayNode`（环形缓冲，延迟量可自动化）、`BiquadFilterNode`（RBJ cookbook 系数；
>   lowpass / highpass / bandpass / lowshelf / highshelf / peaking / notch / allpass +
>   `getFrequencyResponse`）、`AnalyserNode`（FFT + Blackman 窗 + smoothing；时域 / 浮点频域 /
>   字节频域三种读取，均为**就地填充**传入的 TypedArray）。
> - **参数自动化**：`setValueAtTime` / `linearRampToValueAtTime` / `exponentialRampToValueAtTime` /
>   `setTargetAtTime` / `setValueCurveAtTime` / `cancelScheduledValues` / `cancelAndHoldAtTime`；
>   求值**逐样本**（`t = frame / sampleRate`），因此 ramp 的中间值可与浏览器逐点比对。
> - **离线上下文**：`new OfflineAudioContext(channels, length, sampleRate)`（对象形式亦可）→
>   `startRendering()` → `Promise<AudioBuffer>`。时间轴完全由帧号决定 ⇒ **逐样本确定**
>   （同一脚本两次渲染逐字节相同）、不碰设备、可进 CI —— 这是本轮所有数值判据的载体。
> - **实时输出**：`AudioContext` 的 destination 渲染出的交错 float32 PCM 交给宿主注入的
>   `bindings.WGAudioSink`；渲染量随 `currentTime` 流逝补齐（定时器被主循环挤慢时一次补多个
>   量子，音频不会变调），宿主侧（`app/webaudioout.go`）转 s16le → 非阻塞队列 → waveOut，
>   设备**惰性打开**且无设备时静默降级。
>
> **明确不做**（调用即报错，不返回「假节点」）：`AudioWorklet`、`ScriptProcessorNode`、
> `ChannelMergerNode` / `ChannelSplitterNode`（多声道路由）、`PannerNode`（3D）、`ConvolverNode`、
> `DynamicsCompressorNode`、`MediaStreamAudio*`；其中 `createChannelMerger` / `createChannelSplitter`
> 调用即抛 `TypeError`（见 `webaudiograph.go` 文件头取舍与 `webaudio_api.go`）。
>
> **已知限制（如实记录，不假装支持）**：内核按**立体声**工作（`wgChannels = 2`），多声道
> 路由未实现；多个实时 `AudioContext` 同时发声时宿主按到达顺序**串行**写设备（未混音），
> 只有「单实时上下文」在时长上严格正确；`AnalyserNode.fftSize` 固定 2048；
> `getFrequencyResponse` 只回填幅度数组（相位数组未写）。
>
> 落点：`engine/js/bindings/webaudiograph.go`（图内核：参数自动化 / 节点渲染 / 拓扑 / FFT）、
> `engine/js/bindings/webaudio_api.go`（JS 对象层：节点与参数对象 / 离线上下文 / 实时驱动）、
> `engine/js/bindings/webaudio.go`（最小子集，接入图 + 零拷贝 `getChannelData`）、
> `engine/js/jsc/goja_adapter.go`（新增 `Float32ArrayView` 零拷贝视图）、
> `app/webaudio.go`（宿主解码器注入）、`app/webaudioout.go`（宿主实时输出）。
> 测试：`engine/js/bindings/webaudiograph_test.go`（内核直连 4 条）、
> `engine/js/bindings/webaudio_graph_test.go`（JS 层 12 条）、`app/webaudioout_test.go`（宿主输出 6 条）。

## 1. 一句话结论

引擎侧的音频**状态机与播放时钟已经可用**（本轮实测：元数据 `rs=4`、`duration≈1s`、
`play()` 后 `currentTime` 0 → 1 播完），**缺的只是「解码 → 输出设备」这一段**——
而 `goskia` 当前**完全没有音频能力**（无 audio/sound/wave/mixer 任何 API，
只有 Skia 图形 + GLFW 窗口），因此 A3 需要新建一条音频输出链路。

## 2. 现状（第三批实测取证）

| 维度 | 现状 | 证据 |
|---|---|---|
| 元数据（`rs`/`duration`） | ✅ 已通：`wav/mp3/ogg/m4a` 的 file/rel 三来源 `rs=4`、`duration≈1s`（m4a 1.0 / 部分 1.04）；`loadedmetadata` 派发 | 探针 `sine-440-1s.*` 行 12 格 **L1**；`dev/media/out/geom-*.json` |
| 播放时钟（`currentTime`） | ✅ 已通：`play()` 后走 `media_element.go` 的时钟推进，file/rel 从 0 → 1（播完）；`timeupdate`/`ended` 派发 | `geom-Browser.json` 的 `playing` 列表：`rs=4, duration=1, ct=1` |
| 音频**输出**（发声） | ❌ 无：PCM 无去处。`play()` 只推进时钟，没有任何输出调用 | 引擎/`app`/`goskia` 三层均无输出 API |
| WebAudio | ✅ **完整子集已落地**（A3-3）：最小子集（`AudioContext` / `decodeAudioData` 真解码）+ AudioNode 图 / AudioParam 自动化 / `OfflineAudioContext` / 实时输出；不做项与已知限制见文首「实施状态」 | 最小子集：`dev/media/out/report.md` TC-M-603 小节（44100Hz / 1ch / 44100 帧、FFT 主峰 439.95 Hz）；完整子集：`engine/js/bindings/webaudio_graph_test.go` 12 条 + `webaudiograph_test.go` 4 条 + `app/webaudioout_test.go` 6 条（离线渲染逐样本确定、增益/ramp/延迟/声像/分析器数值判据） |
| `data:` 来源的音频 | ⚠️ 仅 `rs=1`、`duration=NaN`：宿主元数据探测按**本地文件路径**走 ffmpeg，`data:` URI 无本地路径 | 探针 `sine-440-1s.* × data` 行 |
| 判据（L4-S） | ⚠️ 未落地：TC-M-604 要求「播放 1s 正弦 → 采回波形 → 频谱主峰 ≈ 440 Hz」 | §5 G6 |

**缺口定位（一张图）**：

```
<audio src> ──①元数据(ffmpeg -i)──✅ 已通（宿主 MediaProbe）
            ──②状态机/时钟─────────✅ 已通（bindings/media_element.go）
            ──③PCM 解码────────────❌ 缺（无处调用）
            ──④输出设备───────────❌ 缺（平台 API，goskia 无）
            ──⑤L4-S 判据───────────❌ 缺（采回波形比对）
```

## 3. 路线选型

| 路线 | 做法 | 优点 | 缺点 |
|---|---|---|---|
| **(a) 宿主注入 PCM + 宿主/引擎输出**（推荐） | 与 A1/A2 的「宿主注入帧流」同构：宿主用 `ffmpeg -i x.wav -f s16le -ac 2 -ar 48000 -` 解出 PCM，引擎按 `(url, 起止时刻)` 取块并做时钟对齐；输出端由 wb-ui 提供**可选**后端或宿主自备 | 引擎不背解码器；与视频路线对称；可先落地「PCM 数据流 + 判据」，输出后端独立演进；无第三方依赖（输出用平台 API） | 宿主必须自带输出能力（否则"静音播放"）；需要设计 PCM 拉取的时序与缓冲 |
| **(b) 引擎内置解码 + 输出**（goskia 扩展） | goskia 新增音频输出绑定（Windows WASAPI / 跨平台 miniaudio/oto），引擎自带解码与混音 | 开箱发声，宿主零负担 | goskia 现在**零音频基础**，需从零建；体积/许可/跨平台构建成本（与 V8 同类的「必须用户确认」项）；引入第三方库需评估许可 |
| (c) 只做状态机 + 波形数据（不发声） | 不新增输出，仅产出 PCM 供宿主自用 | 成本最低 | **不满足决策 4「要真实播放」**，排除 |

**建议**：走 **(a)**，并把「输出后端」拆分成分阶段交付（§7）。理由：
①与主线 A 已落地的视频路线同构（宿主注入 + 引擎只做状态机与呈现）；
②不触碰 goskia 的跨平台构建面；
③即便宿主暂时没有输出设备，A3-1 也能把「PCM 数据流 + 时钟对齐 + 判据」先做实。

## 4. 依赖与工作量拆解（估）

| # | 工作项 | 落点 | 估量 | 依赖 |
|---|---|---|---|---|
| 1 | PCM 通道（宿主注入） | `engine/rendering/audioframe.go`（新，类比 `videoframe.go`）+ `rendering.SetAudioPCMSource` | ~250 行 | 无 |
| 2 | 绑定层接线（play/pause/seek/currentTime → 拉 PCM、时钟以音频为主时钟） | `engine/js/bindings/media_element.go` | ~200 行 | 1 |
| 3 | 宿主 PCM 解码（复用 `MediaProbe` 的 ffmpeg 通道，`-f s16le` 管道 + 缓存） | `app/mediaprobe.go` | ~180 行 | 1 |
| 4 | 输出后端（首选 Windows：`waveOut`/WASAPI；非 Windows 优雅降级为「静音 + 状态标记」） | `app/audioout_windows.go`（新） | ~250 行 | 3 |
| 5 | 判据：TC-M-601/602 采集（探针已有 `rs/duration/ct`）+ TC-M-604 的 PCM 波形/频谱比对 | `cmd/psai/mediaprobe.go`（+ 可选 `mediaaudio_selftest.go`） | ~200 行 | 3,4 |
| 6 | 文档与基线（G6 目标列逐条判定 + 基线更新） | 本文档 + 两份媒体文档 | ~50 行 | 全部 |

**注**：若最终选路线 (b)，则 #4 变为「goskia 音频输出绑定 + wb-ui 输出引擎」（估 600~900 行 + 第三方库引入），#1–#3 仍需（解码/混音落点会变）。

## 5. 验收判据（对应 §5 G6，逐条）

| 用例 | 目标 | 本轮状态 | A3 后应达 |
|---|---|---|---|
| TC-M-601 | `rs=4`、`duration≈1s` | ✅ **已达成**（12 格 L1） | 保持 |
| TC-M-602 | `play()` 后 500ms `currentTime ≈ 0.5` | ✅ **已达成**（时钟推进到 1.00 播完；需补一条"500ms 处 ≈0.5"的定点判据） | 保持 + 定点判据 |
| TC-M-603 | `AudioContext` / `decodeAudioData` 存在 | ✅ **已达成**（A3-3；真解码 `AudioBuffer` + FFT 复核 440Hz） | 保持，并**扩展为完整子集**：AudioContext 的图 / 参数 / 离线渲染 / 实时输出均已可用（不做项见文首「实施状态」） |
| TC-M-604 | 播放 1s 正弦，采回波形，频谱主峰 ≈ 440 Hz | ❌ 未落地 | **L4-S 达成** |

**TC-M-604 的难点与备选判据**：原判据要求「系统输出设备**录音**比对」，需要环回设备
（Windows「立体声混音」常被禁用或不存在）——直接依赖它会让判据在多数机器上无法执行。
建议**两条并行**：

- **判据 A（主，本机可执行）**：宿主的输出回调收到的 PCM 做 FFT，主峰 ≈ 440 Hz，
  且送出的样本时长与播放时钟一致（`samples / sampleRate ≈ currentTime`）。
  这是「引擎确实把音频交给了输出」的**自证**判据——证据强度低于真实录音，但可复现、无需硬件。
- **判据 B（外部复核，可选）**：在**具备环回设备**的机器上，`ffmpeg -f dshow -i audio="立体声混音"`
  录音 1s 后 FFT 比对（无设备则标 `SKIP(no-loopback)`，与 Edge 对照的 `SKIP(no-edge)` 同一处理方式）。

## 6. 风险与不做项

| 风险/项 | 说明 | 处置 |
|---|---|---|
| 平台差异 | Windows / Linux / macOS 输出 API 不同 | 先做 Windows；其余平台走「PCM 回调 + 宿主自备输出」，文档标注支持矩阵 |
| 时钟同步漂移 | 音频硬件时钟与引擎 RAF 时钟不同步 | 规范做法：以**音频时钟为主时钟**，`currentTime` 跟随输出位置（A3-1 设计要点） |
| 环回设备 | 多数机器没有「立体声混音」 | 判据 A 为主（§5），判据 B 可选 |
| 许可与体积 | 若走路线 (b) 引入 miniaudio/oto | 立项时评估；路线 (a) 无第三方依赖 |
| 不做 | WebAudio 的**外围部分**：`AudioWorklet` / `ScriptProcessorNode` / 多声道路由（`ChannelMerger`·`ChannelSplitter`）/ 3D `PannerNode` / `ConvolverNode` / `DynamicsCompressorNode` / `MediaStreamAudio*` | **不承诺**；调用即抛 `TypeError`（不返回假节点）。**图式音频本身已做**（A3-3 完整子集，见文首「实施状态」） |
| 不做 | 音频**编码**（`MediaRecorder`） | 不在 G6 判据内，不投入 |

## 7. 建议分期（供确认）

| 期 | 内容 | 交付判据 |
|---|---|---|
| **A3-1** | PCM 通道 + 宿主 ffmpeg 解码 + Windows 输出后端 | TC-M-602 定点判据 + 判据 A（PCM FFT 主峰 440Hz）→ 音频等级 **L1 → L4**（发声） |
| **A3-2** | 环回设备录音复核（判据 B，可选） | L4-S（外部视角），无设备时 `SKIP(no-loopback)` |
| **A3-3** | WebAudio：最小子集 + **完整子集**（图 / 参数 / 离线渲染 / 实时输出）（**✅ 已实施**，边界与限制见文首「实施状态」） | TC-M-603 ✅ **达成**（并覆盖图式音频的数值判据） |

## 8. 待用户确认的决策点

1. **路线**：(a) 宿主注入 PCM（推荐）还是 (b) goskia 内置输出？
2. **平台范围**：只做 Windows 输出，还是要求三平台同时（后者成本约 ×2~3）？
3. **WebAudio**：✅ **已确认要**（`AudioContext`/`decodeAudioData`）——A3-3 已按此实施：最小子集达成 TC-M-603，随后扩展为**完整子集**（图 / 参数 / 离线渲染 / 实时输出），不做项与已知限制见文首「实施状态」。
4. **L4-S 判据口径**：接受「判据 A 为主 + 判据 B 可选」吗（否则需要一台具备环回设备的机器才能验收）？
5. **是否与「媒体宿主注入通道未受资源策略约束」的修复（见 `media-format-verification-plan.md` §3.4 待决项）合并立项**——两者都动「宿主注入媒体数据」这条链路，合并可省一次接口设计。
