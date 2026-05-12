from flask import Flask, request, send_file, jsonify
from voxcpm import VoxCPM
import soundfile as sf
import io, os, random
import threading

app = Flask(__name__)

# Global model with lazy loading
_model = None
_model_lock = threading.Lock()

def get_model():
    global _model
    if _model is None:
        with _model_lock:
            if _model is None:  # Double-check locking
                print("Loading VoxCPM2 model...")
                # Load model without warmup to avoid the IndexError during initialization
                _model = VoxCPM.from_pretrained("openbmb/VoxCPM2", load_denoiser=False, optimize=False)
                print("Model loaded!")
    return _model

EMOTION_MAP = {
    "neutral":  "A calm and clear voice",
    "happy":    "A cheerful and upbeat voice, warm and smiling",
    "sad":      "A soft and sorrowful voice, slow and heavy",
    "angry":    "An intense and forceful voice, sharp and strong",
    "excited":  "An energetic and enthusiastic voice, fast and lively",
    "calm":     "A gentle and relaxed voice, smooth and soothing"
}

@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok"})

@app.route("/synthesize", methods=["POST"])
def synthesize():
    try:
        data = request.get_json()
        text = data.get("text", "")
        emotion = data.get("emotion", "neutral")
        humanize = data.get("humanize", True)

        model = get_model()
        style = EMOTION_MAP.get(emotion, EMOTION_MAP["neutral"])

        if humanize:
            variation = random.uniform(-0.03, 0.03)
            pace = "slightly faster pace" if variation > 0 else "slightly slower pace"
            style += f", {pace}, with natural breathing pauses"

        styled_text = f"({style}){text}"
        print(f"Synthesizing: {styled_text[:80]}...")

        wav = model.generate(text=styled_text, cfg_value=1.5, inference_timesteps=4, max_len=120)

        buf = io.BytesIO()
        sf.write(buf, wav, model.tts_model.sample_rate, format="WAV")
        buf.seek(0)
        return send_file(buf, mimetype="audio/wav", download_name="output.wav")

    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
        return jsonify({"error": str(e)}), 500

if __name__ == "__main__":
    port = int(os.environ.get("TTS_PORT", 5500))
    app.run(host="0.0.0.0", port=port, threaded=True)
