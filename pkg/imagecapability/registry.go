package imagecapability

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/constant"
)

const (
	configPathEnvironment = "IMAGE_CAPABILITY_CONFIG_FILE"
	defaultConfigFileName = "image-capabilities.json"
	configCheckInterval   = time.Second
)

//go:embed image-capabilities.default.json
var defaultConfigJSON []byte

type configDocument struct {
	Version int              `json:"version"`
	Rules   []capabilityRule `json:"rules"`
}

type capabilityRule struct {
	Name         string         `json:"name"`
	Match        ruleMatch      `json:"match"`
	Capabilities capabilitySpec `json:"capabilities"`
}

type ruleMatch struct {
	ChannelTypes         []int    `json:"channel_types,omitempty"`
	ExcludedChannelTypes []int    `json:"excluded_channel_types,omitempty"`
	ExactModels          []string `json:"exact_models,omitempty"`
	ModelPrefixes        []string `json:"model_prefixes,omitempty"`
	ModelContains        []string `json:"model_contains,omitempty"`
}

type capabilitySpec struct {
	Provider                      string            `json:"provider"`
	SizeMode                      SizeMode          `json:"size_mode"`
	Sizes                         []string          `json:"sizes,omitempty"`
	AspectRatios                  []string          `json:"aspect_ratios,omitempty"`
	Resolutions                   []string          `json:"resolutions,omitempty"`
	Qualities                     []string          `json:"qualities,omitempty"`
	OutputFormats                 []string          `json:"output_formats,omitempty"`
	DefaultSize                   string            `json:"default_size,omitempty"`
	DefaultAspectRatio            string            `json:"default_aspect_ratio,omitempty"`
	DefaultResolution             string            `json:"default_resolution,omitempty"`
	DefaultQuality                string            `json:"default_quality,omitempty"`
	DefaultOutputFormat           string            `json:"default_output_format,omitempty"`
	SupportsEditing               bool              `json:"supports_editing,omitempty"`
	SupportsModeration            bool              `json:"supports_moderation,omitempty"`
	SupportsOutputCompression     bool              `json:"supports_output_compression,omitempty"`
	MaxImages                     int               `json:"max_images"`
	ResolutionSuffixes            map[string]string `json:"resolution_suffixes,omitempty"`
	ResolutionSuffixModelPrefixes []string          `json:"resolution_suffix_model_prefixes,omitempty"`
}

type capabilityRegistry struct {
	mu             sync.RWMutex
	path           string
	rules          []capabilityRule
	defaultConfig  []byte
	loadedFile     bool
	fingerprint    fileFingerprint
	contentHash    [sha256.Size]byte
	hasContentHash bool
	lastChecked    time.Time
	lastError      string
}

type fileFingerprint struct {
	modTime time.Time
	size    int64
}

var defaultRules = mustParseRules(defaultConfigJSON)
var registry = newCapabilityRegistry(runtimeConfigPath(), defaultRules, defaultConfigJSON)

// Resolve returns the first configured capability rule that matches the
// channel and upstream model. It does not change the supplied model name.
func Resolve(channelType int, modelName string) (Capability, bool) {
	return registry.resolve(channelType, modelName)
}

// EnsureRuntimeConfig creates and validates the persistent configuration during
// startup. A load failure is non-fatal because the embedded, versioned defaults
// remain active until a valid file is available.
func EnsureRuntimeConfig() {
	registry.reloadIfNeeded()
}

// GeminiImageResolution is retained for the Gemini relay. Its result comes
// from the same external capability configuration as Playground controls.
func GeminiImageResolution(modelName string) string {
	capability, ok := Resolve(constant.ChannelTypeGemini, modelName)
	if !ok {
		return ""
	}
	resolution, _ := capability.resolutionForModelSuffix(modelName)
	return resolution
}

func (r *capabilityRegistry) resolve(channelType int, modelName string) (Capability, bool) {
	r.reloadIfNeeded()
	normalizedModel := strings.ToLower(strings.TrimSpace(modelName))
	if normalizedModel == "" {
		return Capability{}, false
	}

	r.mu.RLock()
	rules := r.rules
	r.mu.RUnlock()
	for _, rule := range rules {
		if !rule.Match.matches(channelType, normalizedModel) {
			continue
		}
		return ApplyModelAliasDefaults(rule.Capabilities.capability(), modelName), true
	}
	return Capability{}, false
}

// ApplyModelAliasDefaults handles configured model suffix aliases such as
// "-4K". The alias is only interpreted for capability presentation; callers
// retain the original model name for routing and relay.
func ApplyModelAliasDefaults(capability Capability, modelName string) Capability {
	resolution, ok := capability.resolutionForModelSuffix(modelName)
	if !ok {
		return capability
	}
	capability.DefaultResolution = resolution
	capability.Resolutions = nil
	return capability
}

// Intersect returns the conservative capability set shared by all candidate
// channels for a public model.
func Intersect(left Capability, right Capability) Capability {
	provider := left.Provider
	if provider != right.Provider {
		provider = ProviderMixed
	}

	sizeMode := left.SizeMode
	if sizeMode != right.SizeMode {
		sizeMode = SizeModeNone
	}

	result := Capability{
		Provider:                  provider,
		SizeMode:                  sizeMode,
		Sizes:                     intersectValues(left.Sizes, right.Sizes),
		AspectRatios:              intersectValues(left.AspectRatios, right.AspectRatios),
		Resolutions:               intersectValues(left.Resolutions, right.Resolutions),
		Qualities:                 intersectValues(left.Qualities, right.Qualities),
		OutputFormats:             intersectValues(left.OutputFormats, right.OutputFormats),
		SupportsEditing:           left.SupportsEditing && right.SupportsEditing,
		SupportsModeration:        left.SupportsModeration && right.SupportsModeration,
		SupportsOutputCompression: left.SupportsOutputCompression && right.SupportsOutputCompression,
		MaxImages:                 minimumPositive(left.MaxImages, right.MaxImages),
	}
	if sizeMode == SizeModeNone {
		result.Sizes = nil
		result.AspectRatios = nil
		result.Resolutions = nil
	}
	result.DefaultSize = intersectedDefault(left.DefaultSize, right.DefaultSize, result.Sizes)
	result.DefaultAspectRatio = intersectedDefault(left.DefaultAspectRatio, right.DefaultAspectRatio, result.AspectRatios)
	result.DefaultResolution = intersectedDefault(left.DefaultResolution, right.DefaultResolution, result.Resolutions)
	if result.DefaultResolution == "" && sizeMode == SizeModeAspectRatioResolution && len(result.Resolutions) == 0 &&
		left.DefaultResolution != "" && strings.EqualFold(left.DefaultResolution, right.DefaultResolution) {
		result.DefaultResolution = left.DefaultResolution
	}
	result.DefaultQuality = intersectedDefault(left.DefaultQuality, right.DefaultQuality, result.Qualities)
	result.DefaultOutputFormat = intersectedDefault(left.DefaultOutputFormat, right.DefaultOutputFormat, result.OutputFormats)
	return result
}

func (r ruleMatch) matches(channelType int, modelName string) bool {
	if len(r.ChannelTypes) > 0 && !containsInt(r.ChannelTypes, channelType) {
		return false
	}
	if containsInt(r.ExcludedChannelTypes, channelType) {
		return false
	}

	modelMatched := len(r.ExactModels) == 0 && len(r.ModelPrefixes) == 0
	if containsFold(r.ExactModels, modelName) {
		modelMatched = true
	}
	for _, prefix := range r.ModelPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			modelMatched = true
			break
		}
	}
	if !modelMatched {
		return false
	}
	for _, fragment := range r.ModelContains {
		if !strings.Contains(modelName, fragment) {
			return false
		}
	}
	return true
}

func (s capabilitySpec) capability() Capability {
	return Capability{
		Provider:                      s.Provider,
		SizeMode:                      s.SizeMode,
		Sizes:                         append([]string(nil), s.Sizes...),
		AspectRatios:                  append([]string(nil), s.AspectRatios...),
		Resolutions:                   append([]string(nil), s.Resolutions...),
		Qualities:                     append([]string(nil), s.Qualities...),
		OutputFormats:                 append([]string(nil), s.OutputFormats...),
		DefaultSize:                   s.DefaultSize,
		DefaultAspectRatio:            s.DefaultAspectRatio,
		DefaultResolution:             s.DefaultResolution,
		DefaultQuality:                s.DefaultQuality,
		DefaultOutputFormat:           s.DefaultOutputFormat,
		SupportsEditing:               s.SupportsEditing,
		SupportsModeration:            s.SupportsModeration,
		SupportsOutputCompression:     s.SupportsOutputCompression,
		MaxImages:                     s.MaxImages,
		resolutionSuffixes:            cloneStringMap(s.ResolutionSuffixes),
		resolutionSuffixModelPrefixes: append([]string(nil), s.ResolutionSuffixModelPrefixes...),
	}
}

func (c Capability) resolutionForModelSuffix(modelName string) (string, bool) {
	if len(c.resolutionSuffixes) == 0 {
		return "", false
	}
	normalizedModel := strings.ToLower(strings.TrimSpace(modelName))
	if len(c.resolutionSuffixModelPrefixes) > 0 {
		matchesPrefix := false
		for _, prefix := range c.resolutionSuffixModelPrefixes {
			if strings.HasPrefix(normalizedModel, prefix) {
				matchesPrefix = true
				break
			}
		}
		if !matchesPrefix {
			return "", false
		}
	}
	suffixes := make([]string, 0, len(c.resolutionSuffixes))
	for suffix := range c.resolutionSuffixes {
		suffixes = append(suffixes, suffix)
	}
	sort.Slice(suffixes, func(i, j int) bool { return len(suffixes[i]) > len(suffixes[j]) })
	for _, suffix := range suffixes {
		if strings.HasSuffix(normalizedModel, suffix) {
			return c.resolutionSuffixes[suffix], true
		}
	}
	return "", false
}

func newCapabilityRegistry(path string, fallbackRules []capabilityRule, defaultConfig []byte) *capabilityRegistry {
	return &capabilityRegistry{
		path:          path,
		rules:         cloneRules(fallbackRules),
		defaultConfig: append([]byte(nil), defaultConfig...),
	}
}

func runtimeConfigPath() string {
	if path := strings.TrimSpace(os.Getenv(configPathEnvironment)); path != "" {
		return path
	}
	workingDir, err := os.Getwd()
	if err == nil && filepath.Clean(workingDir) == "/data" {
		return filepath.Join(workingDir, defaultConfigFileName)
	}
	return ""
}

func (r *capabilityRegistry) reloadIfNeeded() {
	if r.path == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.lastChecked.IsZero() && time.Since(r.lastChecked) < configCheckInterval {
		return
	}
	r.lastChecked = time.Now()

	info, err := os.Stat(r.path)
	if errors.Is(err, os.ErrNotExist) {
		if r.loadedFile {
			r.logLoadError("configuration file was removed; retaining the last valid configuration")
			return
		}
		if err := writeDefaultConfig(r.path, r.defaultConfig); err != nil {
			r.logLoadError(fmt.Sprintf("cannot create default configuration: %v", err))
			return
		}
		info, err = os.Stat(r.path)
	}
	if err != nil {
		r.logLoadError(fmt.Sprintf("cannot inspect configuration: %v", err))
		return
	}

	data, err := os.ReadFile(r.path)
	if err != nil {
		r.logLoadError(fmt.Sprintf("cannot read configuration: %v", err))
		return
	}
	fingerprint := fileFingerprint{modTime: info.ModTime(), size: info.Size()}
	contentHash := sha256.Sum256(data)
	if r.loadedFile && fingerprint == r.fingerprint && r.hasContentHash && contentHash == r.contentHash {
		return
	}
	rules, err := parseRules(data)
	if err != nil {
		r.logLoadError(fmt.Sprintf("invalid configuration: %v", err))
		return
	}
	r.rules = rules
	r.loadedFile = true
	r.fingerprint = fingerprint
	r.contentHash = contentHash
	r.hasContentHash = true
	r.lastError = ""
	log.Printf("image capability configuration loaded from %s", r.path)
}

func (r *capabilityRegistry) logLoadError(message string) {
	if r.lastError == message {
		return
	}
	r.lastError = message
	log.Printf("image capability configuration %s", message)
}

func writeDefaultConfig(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".image-capabilities-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o640); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryPath, path); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func mustParseRules(data []byte) []capabilityRule {
	rules, err := parseRules(data)
	if err != nil {
		panic(fmt.Sprintf("invalid embedded image capability configuration: %v", err))
	}
	return rules
}

func parseRules(data []byte) ([]capabilityRule, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document configDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var additional any
	if err := decoder.Decode(&additional); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("configuration contains multiple JSON values")
		}
		return nil, err
	}
	if document.Version != 1 {
		return nil, fmt.Errorf("unsupported configuration version %d", document.Version)
	}
	if len(document.Rules) == 0 {
		return nil, errors.New("configuration must contain at least one rule")
	}

	rules := make([]capabilityRule, 0, len(document.Rules))
	for index, rule := range document.Rules {
		if err := rule.normalizeAndValidate(); err != nil {
			return nil, fmt.Errorf("rule %d: %w", index+1, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func (r *capabilityRule) normalizeAndValidate() error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return errors.New("name is required")
	}
	if err := r.Match.normalizeAndValidate(); err != nil {
		return err
	}
	return r.Capabilities.normalizeAndValidate()
}

func (r *ruleMatch) normalizeAndValidate() error {
	r.ExactModels = normalizeStrings(r.ExactModels)
	r.ModelPrefixes = normalizeStrings(r.ModelPrefixes)
	r.ModelContains = normalizeStrings(r.ModelContains)
	if len(r.ExactModels) == 0 && len(r.ModelPrefixes) == 0 && len(r.ModelContains) == 0 {
		return errors.New("at least one model matcher is required")
	}
	return nil
}

func (s *capabilitySpec) normalizeAndValidate() error {
	s.Provider = strings.ToLower(strings.TrimSpace(s.Provider))
	if !containsString([]string{ProviderOpenAI, ProviderXAI, ProviderGemini, ProviderImagen, ProviderOther}, s.Provider) {
		return fmt.Errorf("unsupported provider %q", s.Provider)
	}
	s.SizeMode = SizeMode(strings.TrimSpace(string(s.SizeMode)))
	if !containsString([]string{string(SizeModeNone), string(SizeModeDimensions), string(SizeModeAspectRatioResolution)}, string(s.SizeMode)) {
		return fmt.Errorf("unsupported size_mode %q", s.SizeMode)
	}
	s.Sizes = normalizeDisplayStrings(s.Sizes)
	s.AspectRatios = normalizeDisplayStrings(s.AspectRatios)
	s.Resolutions = normalizeDisplayStrings(s.Resolutions)
	s.Qualities = normalizeDisplayStrings(s.Qualities)
	s.OutputFormats = normalizeDisplayStrings(s.OutputFormats)
	s.DefaultSize = strings.TrimSpace(s.DefaultSize)
	s.DefaultAspectRatio = strings.TrimSpace(s.DefaultAspectRatio)
	s.DefaultResolution = strings.TrimSpace(s.DefaultResolution)
	s.DefaultQuality = strings.TrimSpace(s.DefaultQuality)
	s.DefaultOutputFormat = strings.TrimSpace(s.DefaultOutputFormat)
	if s.MaxImages < 1 {
		return errors.New("max_images must be at least 1")
	}
	if err := validateDefault("default_size", s.DefaultSize, s.Sizes); err != nil {
		return err
	}
	if err := validateDefault("default_aspect_ratio", s.DefaultAspectRatio, s.AspectRatios); err != nil {
		return err
	}
	if err := validateDefault("default_resolution", s.DefaultResolution, s.Resolutions); err != nil {
		return err
	}
	if err := validateDefault("default_quality", s.DefaultQuality, s.Qualities); err != nil {
		return err
	}
	if err := validateDefault("default_output_format", s.DefaultOutputFormat, s.OutputFormats); err != nil {
		return err
	}
	if s.SizeMode == SizeModeDimensions && len(s.AspectRatios) > 0 {
		return errors.New("aspect_ratios require aspect_ratio_resolution size_mode")
	}
	if s.SizeMode == SizeModeDimensions && len(s.Resolutions) > 0 {
		return errors.New("resolutions require aspect_ratio_resolution size_mode")
	}
	if s.SizeMode == SizeModeAspectRatioResolution && len(s.Sizes) > 0 {
		return errors.New("sizes require dimensions size_mode")
	}

	if len(s.ResolutionSuffixes) > 0 {
		normalized := make(map[string]string, len(s.ResolutionSuffixes))
		for suffix, resolution := range s.ResolutionSuffixes {
			suffix = strings.ToLower(strings.TrimSpace(suffix))
			resolution = strings.TrimSpace(resolution)
			if suffix == "" || resolution == "" {
				return errors.New("resolution_suffixes cannot contain empty values")
			}
			normalized[suffix] = resolution
		}
		s.ResolutionSuffixes = normalized
		s.ResolutionSuffixModelPrefixes = normalizeStrings(s.ResolutionSuffixModelPrefixes)
	}
	return nil
}

func validateDefault(field string, value string, values []string) error {
	if value == "" {
		return nil
	}
	if !containsFold(values, value) {
		return fmt.Errorf("%s must be present in its option list", field)
	}
	return nil
}

func normalizeStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func normalizeDisplayStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func cloneRules(rules []capabilityRule) []capabilityRule {
	cloned := make([]capabilityRule, len(rules))
	for index, rule := range rules {
		cloned[index] = capabilityRule{
			Name: rule.Name,
			Match: ruleMatch{
				ChannelTypes:         append([]int(nil), rule.Match.ChannelTypes...),
				ExcludedChannelTypes: append([]int(nil), rule.Match.ExcludedChannelTypes...),
				ExactModels:          append([]string(nil), rule.Match.ExactModels...),
				ModelPrefixes:        append([]string(nil), rule.Match.ModelPrefixes...),
				ModelContains:        append([]string(nil), rule.Match.ModelContains...),
			},
			Capabilities: capabilitySpec{
				Provider:                      rule.Capabilities.Provider,
				SizeMode:                      rule.Capabilities.SizeMode,
				Sizes:                         append([]string(nil), rule.Capabilities.Sizes...),
				AspectRatios:                  append([]string(nil), rule.Capabilities.AspectRatios...),
				Resolutions:                   append([]string(nil), rule.Capabilities.Resolutions...),
				Qualities:                     append([]string(nil), rule.Capabilities.Qualities...),
				OutputFormats:                 append([]string(nil), rule.Capabilities.OutputFormats...),
				DefaultSize:                   rule.Capabilities.DefaultSize,
				DefaultAspectRatio:            rule.Capabilities.DefaultAspectRatio,
				DefaultResolution:             rule.Capabilities.DefaultResolution,
				DefaultQuality:                rule.Capabilities.DefaultQuality,
				DefaultOutputFormat:           rule.Capabilities.DefaultOutputFormat,
				SupportsEditing:               rule.Capabilities.SupportsEditing,
				SupportsModeration:            rule.Capabilities.SupportsModeration,
				SupportsOutputCompression:     rule.Capabilities.SupportsOutputCompression,
				MaxImages:                     rule.Capabilities.MaxImages,
				ResolutionSuffixes:            cloneStringMap(rule.Capabilities.ResolutionSuffixes),
				ResolutionSuffixModelPrefixes: append([]string(nil), rule.Capabilities.ResolutionSuffixModelPrefixes...),
			},
		}
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func intersectValues(left []string, right []string) []string {
	values := make([]string, 0)
	for _, value := range left {
		if containsFold(right, value) {
			values = append(values, value)
		}
	}
	return values
}

func intersectedDefault(left string, right string, values []string) string {
	if left != "" && strings.EqualFold(left, right) && containsFold(values, left) {
		return left
	}
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

func minimumPositive(left int, right int) int {
	if left <= 0 {
		return right
	}
	if right <= 0 || left < right {
		return left
	}
	return right
}
