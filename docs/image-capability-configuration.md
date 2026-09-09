# Playground 图片模型能力配置

Playground 不向上游查询图片模型参数。模型下拉框的尺寸、比例、分辨率、质量和输出格式由
图片能力配置决定，再由 `/api/user/image-models` 返回给前端。

首次启动后，服务会在持久化数据目录创建：

```text
/data/image-capabilities.json
```

本生产部署将 `/opt/qlh-main/data/new-api` 挂载到容器 `/data`，因此实际编辑路径为：

```text
/opt/qlh-main/data/new-api/image-capabilities.json
```

也可以通过 `IMAGE_CAPABILITY_CONFIG_FILE` 指定其他绝对路径。服务每秒最多检查一次文件；
写入有效 JSON 后无需重启或构建镜像，下一次模型能力请求即可使用新规则。

## 编辑原则

- 规则按 `rules` 数组顺序匹配，首个匹配规则生效。精确模型规则应放在前缀规则之前。
- `model` 仅用于匹配，绝不改写、截断或转换用户提交给上游的模型名称。
- `channel_types` 是可选白名单，`excluded_channel_types` 是可选黑名单，数值与渠道类型一致。
- 模型匹配支持 `exact_models`、`model_prefixes` 和 `model_contains`；数组中的匹配不区分大小写。
- `size_mode` 只能是 `dimensions`、`aspect_ratio_resolution` 或 `none`。
- `dimensions` 使用 `sizes`；`aspect_ratio_resolution` 使用 `aspect_ratios` 和 `resolutions`。默认值必须存在于相应选项列表。
- `resolution_suffixes` 可把模型名末尾的 `-1k`、`-2k`、`-4k` 显示为固定分辨率。应同时用
  `resolution_suffix_model_prefixes` 限定可应用的模型前缀，避免把无关别名误判为固定规格。

服务使用最后一份有效配置。JSON 格式或字段校验失败时，会写入错误日志，但继续使用上一版，
不会让已有图片模型从页面消失。

## 新增模型示例

下面规则将公开模型名 `my-image-model-4k` 限定为一个原生 4K OpenAI 兼容模型。将它置于较宽泛的
`gpt-image-` 或其他前缀规则之前：

```json
{
  "name": "my-image-model-4k",
  "match": {
    "channel_types": [1],
    "exact_models": ["my-image-model-4k"]
  },
  "capabilities": {
    "provider": "openai",
    "size_mode": "dimensions",
    "sizes": ["3840x2160", "2160x3840"],
    "default_size": "3840x2160",
    "qualities": ["high"],
    "default_quality": "high",
    "output_formats": ["png"],
    "default_output_format": "png",
    "supports_editing": true,
    "max_images": 1
  }
}
```

修改前先校验 JSON，再使用同目录临时文件替换，避免服务读到半份内容：

```bash
jq . /opt/qlh-main/data/new-api/image-capabilities.json > /opt/qlh-main/data/new-api/image-capabilities.json.next
mv /opt/qlh-main/data/new-api/image-capabilities.json.next /opt/qlh-main/data/new-api/image-capabilities.json
```

配置文件中的全部字段和现有默认规则可直接参考启动后生成的
`/opt/qlh-main/data/new-api/image-capabilities.json`。
