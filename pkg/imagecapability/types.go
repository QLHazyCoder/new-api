package imagecapability

type SizeMode string

const (
	SizeModeNone                  SizeMode = "none"
	SizeModeDimensions            SizeMode = "dimensions"
	SizeModeAspectRatioResolution SizeMode = "aspect_ratio_resolution"
)

const (
	ProviderOpenAI = "openai"
	ProviderXAI    = "xai"
	ProviderGemini = "gemini"
	ProviderImagen = "imagen"
	ProviderOther  = "other"
	ProviderMixed  = "mixed"
)

type Capability struct {
	Provider                  string
	SizeMode                  SizeMode
	Sizes                     []string
	AspectRatios              []string
	Resolutions               []string
	Qualities                 []string
	OutputFormats             []string
	DefaultSize               string
	DefaultAspectRatio        string
	DefaultResolution         string
	DefaultQuality            string
	DefaultOutputFormat       string
	SupportsEditing           bool
	SupportsModeration        bool
	SupportsOutputCompression bool
	MaxImages                 int
}
