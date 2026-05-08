package tts

import "context"

type ctxKey string

const (
	ctxEmotionKey  ctxKey = "tts_emotion"
	ctxHumanizeKey ctxKey = "tts_humanize"
	ctxSpeedKey    ctxKey = "tts_speed"
	ctxPitchKey    ctxKey = "tts_pitch"
)

type VoiceOptions struct {
	Emotion  string
	Humanize *bool
	Speed    float64
	Pitch    float64
}

func WithEmotion(ctx context.Context, emotion string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxEmotionKey, emotion)
}

func WithHumanize(ctx context.Context, humanize bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxHumanizeKey, humanize)
}

func WithSpeed(ctx context.Context, speed float64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxSpeedKey, speed)
}

func WithPitch(ctx context.Context, pitch float64) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, ctxPitchKey, pitch)
}

func OptionsFromContext(ctx context.Context, defaults VoiceOptions) VoiceOptions {
	out := defaults
	if ctx == nil {
		return out
	}
	if v, ok := ctx.Value(ctxEmotionKey).(string); ok && v != "" {
		out.Emotion = v
	}
	if v, ok := ctx.Value(ctxHumanizeKey).(bool); ok {
		out.Humanize = &v
	}
	if v, ok := ctx.Value(ctxSpeedKey).(float64); ok && v > 0 {
		out.Speed = v
	}
	if v, ok := ctx.Value(ctxPitchKey).(float64); ok && v > 0 {
		out.Pitch = v
	}
	return out
}
