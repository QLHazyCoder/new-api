package dto

type ImageModelCapabilities struct {
	Provider                  string   `json:"provider"`
	SizeMode                  string   `json:"size_mode"`
	Sizes                     []string `json:"sizes"`
	AspectRatios              []string `json:"aspect_ratios"`
	Resolutions               []string `json:"resolutions"`
	Qualities                 []string `json:"qualities"`
	OutputFormats             []string `json:"output_formats"`
	DefaultSize               string   `json:"default_size,omitempty"`
	DefaultAspectRatio        string   `json:"default_aspect_ratio,omitempty"`
	DefaultResolution         string   `json:"default_resolution,omitempty"`
	DefaultQuality            string   `json:"default_quality,omitempty"`
	DefaultOutputFormat       string   `json:"default_output_format,omitempty"`
	SupportsEditing           bool     `json:"supports_editing"`
	SupportsModeration        bool     `json:"supports_moderation"`
	SupportsOutputCompression bool     `json:"supports_output_compression"`
	MaxImages                 int      `json:"max_images"`
}

type UserImageModelOption struct {
	Label        string                 `json:"label"`
	Value        string                 `json:"value"`
	Capabilities ImageModelCapabilities `json:"capabilities"`
}

type UserImageGroupOption struct {
	Label  string                 `json:"label"`
	Value  string                 `json:"value"`
	Ratio  any                    `json:"ratio"`
	Desc   string                 `json:"desc"`
	Models []UserImageModelOption `json:"models"`
}
