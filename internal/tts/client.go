package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Client interface {
	Synthesize(ctx context.Context, text, voice, language, outDir string) (string, error)
	ListVoices(ctx context.Context) ([]Voice, error)
	Preview(ctx context.Context, text, voice, language string) ([]byte, error)
}

type Voice struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	LanguageCode string `json:"language_code"`
	Quality      string `json:"quality"`
}

type HTTPClient struct {
	baseURL   string
	synthPath string
	http      *http.Client
	defaults  VoiceOptions
}

func NewHTTPClient(baseURL, synthPath string, timeout time.Duration, defaults VoiceOptions) *HTTPClient {
	if synthPath == "" {
		synthPath = "/synthesize"
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &HTTPClient{
		baseURL:   strings.TrimRight(baseURL, "/"),
		synthPath: synthPath,
		http:      &http.Client{Timeout: timeout},
		defaults:  defaults,
	}
}

type synthRequest struct {
	Text       string `json:"text"`
	VoiceKey   string `json:"voice_key,omitempty"`
	SpeakerID  string `json:"speaker_id,omitempty"`
	LanguageID string `json:"language_id,omitempty"`
	Emotion    string `json:"emotion,omitempty"`
	Speed      float64 `json:"speed,omitempty"`
	Pitch      float64 `json:"pitch,omitempty"`
	Humanize   *bool   `json:"humanize,omitempty"`
}

func (c *HTTPClient) Synthesize(ctx context.Context, text, voice, language, outDir string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text cannot be empty")
	}
	if c.baseURL == "" {
		return "", fmt.Errorf("tts base URL cannot be empty")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create output dir: %w", err)
	}

	fileName := fmt.Sprintf("voiceover-%d.wav", time.Now().UnixNano())
	outPath := filepath.Join(outDir, fileName)

	audioBytes, err := c.synthesizeBytes(ctx, text, voice, language)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(outPath, audioBytes, 0o644); err != nil {
		return "", fmt.Errorf("write tts output: %w", err)
	}

	return outPath, nil
}

func (c *HTTPClient) Preview(ctx context.Context, text, voice, language string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		text = "This is a voice preview from VoxCPM text to speech."
	}
	return c.synthesizeBytes(ctx, text, voice, language)
}

func (c *HTTPClient) synthesizeBytes(ctx context.Context, text, voice, language string) ([]byte, error) {
	if text == "" {
		return nil, fmt.Errorf("text cannot be empty")
	}
	if c.baseURL == "" {
		return nil, fmt.Errorf("tts base URL cannot be empty")
	}

	options := OptionsFromContext(ctx, c.defaults)
	payload, err := json.Marshal(synthRequest{
		Text:       text,
		VoiceKey:   voice,
		SpeakerID:  voice,
		LanguageID: language,
		Emotion:    strings.TrimSpace(options.Emotion),
		Speed:      options.Speed,
		Pitch:      options.Pitch,
		Humanize:   options.Humanize,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tts request: %w", err)
	}

	doer := c.http
	if timeout := estimatedSynthesisTimeout(text, c.http.Timeout); timeout > c.http.Timeout {
		clone := *c.http
		clone.Timeout = timeout
		doer = &clone
	}

	jsonURL := c.baseURL + c.synthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jsonURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build tts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/wav, application/octet-stream")

	resp, err := doer.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tts request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("tts service returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	defer resp.Body.Close()

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read tts response: %w", err)
	}
	if len(audioBytes) == 0 {
		return nil, fmt.Errorf("tts response body was empty")
	}
	return audioBytes, nil
}

func estimatedSynthesisTimeout(text string, base time.Duration) time.Duration {
	if base <= 0 {
		base = 30 * time.Second
	}

	words := len(strings.Fields(text))
	estimate := 30*time.Second + time.Duration(words)*120*time.Millisecond
	if estimate < base {
		estimate = base
	}
	if estimate > 8*time.Minute {
		estimate = 8 * time.Minute
	}
	return estimate
}

func (c *HTTPClient) ListVoices(ctx context.Context) ([]Voice, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("tts base URL cannot be empty")
	}

	jsonReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/voices", nil)
	if err == nil {
		jsonResp, jsonErr := c.http.Do(jsonReq)
		if jsonErr == nil {
			defer jsonResp.Body.Close()
			if jsonResp.StatusCode >= 200 && jsonResp.StatusCode < 300 {
				var parsed struct {
					Voices []struct {
						Key               string `json:"key"`
						Name              string `json:"name"`
						LanguageCode      string `json:"languageCode"`
						LanguageCodeSnake string `json:"language_code"`
						Quality           string `json:"quality"`
					} `json:"voices"`
				}
				if decodeErr := json.NewDecoder(io.LimitReader(jsonResp.Body, 4*1024*1024)).Decode(&parsed); decodeErr == nil {
					voices := make([]Voice, 0, len(parsed.Voices))
					for _, v := range parsed.Voices {
						key := strings.TrimSpace(v.Key)
						if key == "" {
							continue
						}
						language := strings.TrimSpace(v.LanguageCode)
						if language == "" {
							language = strings.TrimSpace(v.LanguageCodeSnake)
						}
						name := strings.TrimSpace(v.Name)
						if name == "" {
							name = key
						}
						voices = append(voices, Voice{
							Key:          key,
							Name:         name,
							LanguageCode: language,
							Quality:      strings.TrimSpace(v.Quality),
						})
					}
					sort.Slice(voices, func(i, j int) bool {
						if voices[i].LanguageCode == voices[j].LanguageCode {
							if voices[i].Name == voices[j].Name {
								return voices[i].Quality < voices[j].Quality
							}
							return voices[i].Name < voices[j].Name
						}
						return voices[i].LanguageCode < voices[j].LanguageCode
					})
					return voices, nil
				}
			}
		}
	}

	return []Voice{}, nil
}

func parseSelectOptions(htmlText, selectID string) []string {
	selectRe := regexp.MustCompile(`(?is)<select[^>]*id=["']` + regexp.QuoteMeta(selectID) + `["'][^>]*>(.*?)</select>`)
	selectMatch := selectRe.FindStringSubmatch(htmlText)
	if len(selectMatch) < 2 {
		return nil
	}

	optionRe := regexp.MustCompile(`(?is)<option[^>]*value=["']([^"']+)["'][^>]*>(.*?)</option>`)
	optionMatches := optionRe.FindAllStringSubmatch(selectMatch[1], -1)
	values := make([]string, 0, len(optionMatches))
	seen := map[string]struct{}{}
	for _, m := range optionMatches {
		if len(m) < 2 {
			continue
		}
		v := strings.TrimSpace(html.UnescapeString(m[1]))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		values = append(values, v)
	}
	return values
}

func inferLanguageFromVoiceKey(key string) string {
	parts := strings.Split(key, "-")
	if len(parts) == 0 {
		return ""
	}
	lang := strings.TrimSpace(parts[0])
	langRe := regexp.MustCompile(`^[a-z]{2}(?:_[A-Z]{2})?$`)
	if langRe.MatchString(lang) {
		return lang
	}
	return ""
}

func inferQualityFromVoiceKey(key string) string {
	parts := strings.Split(key, "-")
	if len(parts) < 2 {
		return ""
	}
	q := strings.ToLower(strings.TrimSpace(parts[len(parts)-1]))
	switch q {
	case "x_low", "low", "medium", "high":
		return q
	default:
		return ""
	}
}
