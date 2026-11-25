# FlowVision Windows 缩略图后端 spike

- 日期：2026-11-26
- 目标：验证 WIC、Shell、EXIF 内嵌缩略图和纯 Go DCT 四条路线的速度、质量与回退边界
- 结论：WIC 进入默认路径；Shell 只做格式扩展；EXIF 内嵌做 preview；DCT 归档

## 1. 实现方式

### WIC

`thumb_wic_windows.go` 用手写 COM shim 调用 `IWICImagingFactory`，没有引入 `windigo`，`go.mod` 不变。

- `CoInitializeEx(MTA)` + `runtime.LockOSThread`，每个任务释放 decoder / frame / scaler。
- `CreateDecoderFromFilename` 读取文件，`GetFrame(0)` 取第一帧。
- 优先 `IWICBitmapDecoder::GetThumbnail`；内嵌缩略图达到目标尺寸 0.75x 才作为最终图，否则解码完整帧。
- 没有内嵌时用 `IWICBitmapScaler`，`WICBitmapInterpolationModeHighQualityCubic`。
- 支持 24bppBGR/RGB、32bppBGR/BGRA/PBGRA、8bppGray；统一转 `image.RGBA`，预乘 alpha 后交给现有 `thumbnailImage` 合成中性底色。
- EXIF 方向优先复用 `Item.Orient`，未知时再读 APP1。

最终 vtable 索引（已用真实文件验证）：

| 接口 | 方法 | 索引 |
|---|---|---:|
| `IWICImagingFactory` | `CreateDecoderFromFilename` | 3 |
| `IWICImagingFactory` | `CreateBitmapScaler` | 11 |
| `IWICBitmapDecoder` | `GetThumbnail` | 11 |
| `IWICBitmapDecoder` | `GetFrame` | 13 |
| `IWICBitmapScaler` | `Initialize` | 8 |
| `IWICBitmapSource` | `GetSize` | 3 |
| `IWICBitmapSource` | `GetPixelFormat` | 4 |
| `IWICBitmapSource` | `CopyPixels` | 7 |

### Shell

`thumb_shell_windows.go` 通过 `SHCreateItemFromParsingName` 获取 `IShellItemImageFactory`，`GetImage` 后 `GetObject` + `GetDIBits` 转 32bpp BGRA。

- `SIIGBF_THUMBNAILONLY | SIIGBF_BIGGERSIZEOK` 用于普通图片。
- `.ico` 使用 `SIIGBF_ICONONLY | SIIGBF_BIGGERSIZEOK`。
- 只在 HEIC/HEIF/AVIF/JXL/ICO/RAW 等 WIC 覆盖不足的扩展上优先，普通 JPEG 不依赖 Shell provider。
- 失败立即回 Go，不阻塞 worker。

### EXIF 内嵌缩略图

- `meta.go` 从 IFD0 的 next IFD 进入 IFD1，读取 `0x0103` 压缩类型、`0x0201` offset、`0x0202` length。
- 只接受 JPEG 压缩（`0x0103=6`），长度上限 2 MiB，offset/length 必须落在 APP1 范围内。
- `probeImageDims` 一次 open 完成 `DecodeConfig` + EXIF 解析，`Item.HasThumb` 标记是否有可用 IFD1。
- 24 张合成语料中 12 张带内嵌缩略图；`img-000.jpg` 的内嵌 JPEG 约 6266 bytes。

### 纯 Go DCT

`thumb_dct.go` 实现 baseline sequential JPEG 的 DC-only 1/8 解码：解析 DQT/SOF0/DHT/SOS/DRI，Huffman 只取 DC、跳过 AC，输出 1/8 尺寸 YCbCr/Gray，再交给 `thumbnailImage`。

- 支持 4:2:0 / 4:2:2 / 4:4:4 的常见 sampling。
- progressive / arithmetic / CMYK 直接回退。
- 仅作为研究开关 `FLOWVISION_THUMB_BACKEND=dct`，不进入默认路径。

## 2. 性能

同一语料、6 worker、目标高度 220；数据来自 `docs/thumb-bench-wic.json` 和 `docs/thumb-bench-go.json`。

| 目录 | Go | WIC（HighQualityCubic） | 倍数 |
|---|---:|---:|---:|
| 24 张 | 1812 ms | 964 ms | 1.88x |
| 100 张 | 7283 ms | 3402 ms | 2.14x |
| 300 张 | 21287 ms | 10026 ms | 2.12x |

峰值堆：Go 376/419/419 MB；WIC 9/13/15 MB。

早期 Fant 插值版本为 326/1268/3874 ms（5.5x），但真实照片平均通道误差偏大；最终选择 HighQualityCubic，以 2.1x 换取更接近旧路径的质量。WIC 单张 24 MP JPEG 约 33–40 ms，仍显著低于 Go 的 71–75 ms。

真实应用首屏（220 px、4 列、12 张可见卡片，清空磁盘缓存）：24/100/300 分别为 305/376/374 ms；旧 Go 为 749/965/686 ms。全部预取完成 WIC 为 952/1002/998 ms，旧 Go 为 3243/3110/2155 ms。

## 3. 质量

`TestBackendQuality` 对 8 张真实照片在 220 高度下比较 WIC 与旧 Go 输出（JPEG q84 后解码，同尺寸逐像素）：

| 样本 | PSNR | RMSE | MAE |
|---|---:|---:|---:|
| real-01 | 35.1 dB | 4.5 | 3.1 |
| real-02 | 37.1 dB | 3.6 | 2.3 |
| real-03 | 41.5 dB | 2.2 | 1.6 |
| real-04 | 37.2 dB | 3.5 | 2.3 |
| real-05 | 42.0 dB | 2.0 | 1.4 |
| real-06 | 37.2 dB | 3.5 | 2.4 |
| real-07 | 35.9 dB | 4.1 | 3.0 |
| real-08 | 36.9 dB | 3.6 | 2.3 |

平均 MAE 2.35/255，接近计划中“平均通道误差 ≤ 2/255”的暂定线；差异主要来自 WIC 与 Go 的 YCbCr 转换/缩放核，而不是方向或色偏。实机并排看没有明显偏色、拉伸或“低清图被当成最终图”。宽色域/ICC 尚未做像素级验证，发布后保留为回归项。

## 4. 格式矩阵

`TestFormatRender` 与实机截图结果：

| 样本 | 结果 |
|---|---|
| `baseline.jpg` / `progressive.jpg` | 通过 |
| `cmyk.jpg` / `gray.jpg` | 通过 |
| `alpha.png` / `gray16.png` | 通过；透明合成到 `#2a2a2e` |
| `still.gif` / `still.bmp` / `still.tiff` / `still.webp` | 通过 |
| `still.ico` | PNG-compressed ICO 由 `thumb_ico.go` 解码；DIB-only ICO 交给 Shell |
| `corrupt.jpg` / `empty.jpg` | 解码失败，App 画占位，不崩溃 |
| `still.heic` | 当前机器无系统 HEIC codec；WIC/Shell/Go 均失败，显示占位 |
| `raw-iphone.dng` | 系统 RAW Shell provider 出图，测试约 238 ms |
| EXIF orientation 1–8 | 1–4 输出横向、5–8 输出纵向；`TestCorpusOrientation` 通过 |

`.ico` 早期走 Shell 时在当前 Windows build 上被拒绝；增加 PNG-compressed ICO 解析后格式矩阵稳定出图。

## 5. 决策

- **Gate 1**：WIC 单张/整体 ≥1.8x 且质量可接受 → 通过，`auto` 默认 WIC。
- **Shell**：只在 HEIC/RAW/ICO 等格式优先；普通 JPEG 不默认走 Shell。
- **Gate 2**：两阶段首屏 0.31–0.38 s，旧 Go 0.69–0.97 s；全部预取 0.95–1.0 s，旧 Go 2.2–3.2 s。保留两阶段，preview 只对 `HasThumb` 的 JPEG 生效。
- **Gate 3**：DCT 单张 216 ms vs WIC 约 33–40 ms（约 5x 慢），归档为研究开关，不进入默认/回退链。
- **Gate 4**：10 轮滚动 GDI 稳定 46–47、句柄稳定；480 桶字节缓存 233.7 MB；见 `docs/缩略图性能报告.md`。
