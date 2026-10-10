# MediaStudio 端到端媒体验证报告

- 生成时间：2026-10-10 15:28:46
- 应用：`cmd/mediastudio`（录屏剪辑台；界面由 wb-ui 引擎渲染）
- 视口：1280×860 · 录制参数：3.0s @10fps · 音轨=true
- ffmpeg：ffmpeg version N-109957-g373ef1c4fa-20230302 Copyright (c) 2000-2023 the FFmpeg developers
- 素材目录：dev\media\samples

## 一、链路判据

| # | 应用功能 | 用到的媒体能力 | 实测 | 结论 |
|---|---|---|---|---|
| 1 | 界面渲染 | 引擎布局/绘制/CSS 动画/图片解码 | ui.png 52471 字节 | ✅ 通过 |
| 2 | 录屏抓帧 | `WebView.Render()`（预乘 RGBA → PNG） | 30 帧（期望 30）· 最小帧 20803 字节 · 舞台区域首末帧变化像素占比 0.95% | ✅ 通过 |
| 3 | 画面推进 | 引擎 CSS 动画 + 事件循环推进 | 舞台区域首末帧变化像素占比 0.95%（30 帧间隔；静止基线 <0.1%） | ✅ 通过 |
| 4 | 环回录音 | WASAPI loopback + 输出设备播放测试音 | 654720 字节 PCM（48000Hz 2ch，约 3.41s）→ rec.wav 654764 字节 | ✅ 通过 |
| 5 | 视频合成 | ffmpeg（宿主编码侧） | record.mp4 69868 字节 · 舞台区域 (253,90)-(995,752) | ✅ 通过 |
| 6 | 剪辑 | 自持帧序列区间 + 音轨输入级裁剪 | clip.mp4 49996 字节 · 20 帧 · 2.0s | ✅ 通过 |
| 7 | 帧导出 | PNG / JPEG 编码（标准库） | frame-now.png 69046 字节 · frame-now.jpg 105102 字节 | ✅ 通过 |
| 8 | 回放（引擎 <video>） | 宿主元数据 + 按时刻抽帧 | duration=3.00s videoWidth=742 readyState=4 currentTime=1.32s | ✅ 通过 |
| 9 | 回放一致性 | 录制中间帧 vs 回放截图 | 16×16 网格平均色差 6.51（0-255 尺度，越小越像） | ✅ 通过 |

## 二、产物

| 产物 | 路径 | 大小 | 说明 |
|---|---|---|---|
| record.mp4 | `record.mp4` | 69868 字节 | 30 帧 @10fps · 3.0s |
| rec.wav | `_temp/mediastudio/rec.wav` | 654764 字节 | 48000Hz 2ch · 3.0s |
| frame-mid.png | `_temp/mediastudio/frames/f0015.png` | 22039 字节 | 742x662 |
| clip.mp4 | `clip.mp4` | 49996 字节 | 20 帧 · 2.0s（0.5→2.5s） |
| frame-now.png | `frame-now.png` | 69046 字节 | 1280x860 |
| frame-now.jpg | `frame-now.jpg` | 105102 字节 | 1280x860 |
| replay.png | `replay.png` | 71877 字节 | 回放截图 |

中间帧序列落在 `_temp/mediastudio`（不入库，与 `_temp` 同规矩）。

## 三、容器校验（ffprobe）

**record.mp4**

```
Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'dev/media/out/studio/record.mp4':
  Metadata:
    major_brand     : isom
    minor_version   : 512
    compatible_brands: isomiso2avc1mp41
    encoder         : Lavf60.4.100
  Duration: 00:00:03.00, start: 0.000000, bitrate: 186 kb/s
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p(progressive), 742x662, 50 kb/s, 10 fps, 10 tbr, 10240 tbn (default)
    Metadata:
      handler_name    : VideoHandler
      vendor_id       : [0][0][0][0]
      encoder         : Lavc60.6.100 libx264
  Stream #0:1[0x2](und): Audio: aac (LC) (mp4a / 0x6134706D), 48000 Hz, stereo, fltp, 127 kb/s (default)
    Metadata:
      handler_name    : SoundHandler
      vendor_id       : [0][0][0][0]
At least one output file must be specified
```

**clip.mp4**

```
Input #0, mov,mp4,m4a,3gp,3g2,mj2, from 'dev/media/out/studio/clip.mp4':
  Metadata:
    major_brand     : isom
    minor_version   : 512
    compatible_brands: isomiso2avc1mp41
    encoder         : Lavf60.4.100
  Duration: 00:00:02.00, start: 0.000000, bitrate: 199 kb/s
  Stream #0:0[0x1](und): Video: h264 (High) (avc1 / 0x31637661), yuv420p(progressive), 742x662, 62 kb/s, 10 fps, 10 tbr, 10240 tbn (default)
    Metadata:
      handler_name    : VideoHandler
      vendor_id       : [0][0][0][0]
      encoder         : Lavc60.6.100 libx264
  Stream #0:1[0x2](und): Audio: aac (LC) (mp4a / 0x6134706D), 48000 Hz, stereo, fltp, 128 kb/s (default)
    Metadata:
      handler_name    : SoundHandler
      vendor_id       : [0][0][0][0]
At least one output file must be specified
```

**rec.wav**

```
[aist#0:0/pcm_s16le @ 00000238262da0c0] Guessed Channel Layout: stereo
Input #0, wav, from '_temp/mediastudio/rec.wav':
  Duration: 00:00:03.41, bitrate: 1536 kb/s
  Stream #0:0: Audio: pcm_s16le ([1][0][0][0] / 0x0001), 48000 Hz, 2 channels, s16, 1536 kb/s
At least one output file must be specified
```

## 四、回放观测

- 事件序列：`seeked`
- duration=3.00s · videoWidth=742 · videoHeight=662 · currentTime=1.32s · readyState=4
- 预览区矩形：(1028,90)-(1264,240)
- 回放截图：`replay.png`；对比用录制中间帧：`_temp/mediastudio/frames/f0015.png`

## 五、结论

**9/9 项判据通过**：应用完成了「录屏 + 录音 → 合成 mp4 → 剪辑 → 导出帧 → 回放比对」的完整闭环，这一闭环的每一步都压在本仓媒体链路上，因此可判定媒体能力端到端可用。

## 六、应用日志

```
无头自动演示：视口 1280x860，录制 3.0s @10fps，音轨=true
① 界面渲染截图：dev\media\out\studio\ui.png
开始录制：(253,90)-(995,752)（3.0s @ 10fps，音轨=true）
布局自上报 {"tag":"t+800ms","win":"1280x860","doc":"1280x860","body":"1280x860","status":"录制中","assets":"1","rects":".work=0,52,1280x716 | .panel.ctrl=0,52,236x716 | .panel.stagep=237,52,774x716 | .panel.assets=1012,52,268x716 | #stage=253,90,742x662 | .timeline=0,768,1280x92"}
布局自上报 {"tag":"t+3000ms","win":"1280x860","doc":"1280x860","body":"1280x860","status":"录制中","assets":"1","rects":".work=0,52,1280x716 | .panel.ctrl=0,52,236x716 | .panel.stagep=237,52,774x716 | .panel.assets=1012,52,268x716 | #stage=253,90,742x662 | .timeline=0,768,1280x92"}
抓帧完成：30 帧，首末帧变化像素占比 0.95%（舞台区域）
环回录音：654720 字节 PCM（48000Hz 2ch，3.41s）
合成完成：record.mp4（69868 字节）
点击 #btn-record @ (110.5, 256.0)
② 录制完成：30 帧 / 69868 字节 / 音轨 654720 字节 / 首末帧差 0.95
剪辑完成：clip.mp4（20 帧，0.5s→2.5s，49996 字节）
点击 #btn-export-clip @ (110.5, 492.5)
③ 剪辑完成：20 帧 · 2.0s · 49996 字节
导出当前帧：dev\media\out\studio\frame-now.png
点击 #btn-frames @ (110.5, 538.5)
导出当前帧：dev\media\out\studio\frame-now.jpg
点击 #btn-frame-jpg @ (110.5, 584.5)
④ 帧导出：dev\media\out\studio\frame-now.png / dev\media\out\studio\frame-now.jpg
点击 #btn-play @ (1146.0, 271.0)
回放验证：src=file:///F:/syproject/wb-ui/dev/media/out/studio/record.mp4 duration=3.00s videoWidth=742 readyState=4 networkState=1 errorCode=0 currentTime=1.32 事件=seeked 签名差=6.51
```
