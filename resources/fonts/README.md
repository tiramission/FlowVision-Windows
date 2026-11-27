# 界面字体（可选）

把 MiSans 的**官方原文件**放进这个目录，构建时会被 `mygo build` 装到
`resources/fonts`，应用启动时注册并以 `MiSans` 作为界面字体。目录为空或没有可用
文件时，应用继续使用系统字体，界面上看不出任何区别。

推荐直接放官方的可变字体，一个文件覆盖 100–900 全部字重：

```text
resources/fonts/
  MiSans VF.ttf          20 MB   （官方 MiSans/MiSans VF.ttf）
```

也可以用静态字重（`MiSans-Regular/Medium/Semibold/Bold.otf`，每个约 6.5 MB），但
必须把界面用到的 400 / 500 / 600 / 700 都放齐，缺档会让那些文字回退到别的字体。

## 为什么文件不在仓库里

MiSans 的许可协议（https://hyperos.mi.com/font/zh/download/ 页面底部）要求：

- 在软件中注明使用了 MiSans 字体 —— 关于窗口和 `THIRD_PARTY_NOTICES.md` 已包含。
- **不得对字体进行改编或二次开发** —— 因此不能子集化、不能转 woff2、不能合并
  CDN 上的分片；只能用官方发布的完整字体文件。
- 不得**单独**分发字体软件或其副本 —— 随应用一起分发属于灰区，请自行评估。
  仓库默认不包含字体文件；如果要让 GitHub Actions 打出的安装包也带 MiSans，
  需要把这些文件提交进仓库。

## 体积

`MiSans VF.ttf` 约 20 MB。安装包从 6 MB 涨到 20 MB 左右，EXE 本体不变。
