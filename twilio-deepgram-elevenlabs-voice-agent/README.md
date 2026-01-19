# Twilio + Deepgram + ElevenLabs Voice Agent

A complete voice agent example demonstrating real-time speech-to-text and text-to-speech over telephony.

## Architecture

```
┌──────────┐        ┌─────────────────┐         ┌────────────────────────────────┐
│  Caller  │◄──────►│     Twilio      │◄───────►│         OmniVoice              │
│  (PSTN)  │  PSTN  │     Media       │WebSocket│                                │
└──────────┘        │     Streams     │ (μ-law) │  ┌─────────┐     ┌──────────┐  │
                    └─────────────────┘         │  │Deepgram │     │ElevenLabs│  │
                                                │  │  STT    │────►│   TTS    │  │
                                                │  └────┬────┘     └────┬─────┘  │
                                                │       │               │        │
                                                │       ▼               │        │
                                                │  ┌─────────────────┐  │        │
                                                │  │   Agent Logic   │──┘        │
                                                │  │  (echo/LLM)     │           │
                                                │  └─────────────────┘           │
                                                └────────────────────────────────┘
```

## Flow

1. Caller dials Twilio phone number
2. Twilio connects via Media Streams (mu-law audio)
3. Audio goes to Deepgram STT → transcripts
4. Transcripts are processed by agent logic
5. Response goes to ElevenLabs TTS → audio
6. Audio (ulaw) streams back to caller via Twilio

## Features

- **Real-time STT**: Deepgram Nova-2 model with interim results
- **Low-latency TTS**: ElevenLabs Turbo v2.5 with native mu-law output
- **Barge-in support**: TTS stops when user starts speaking
- **Turn-taking**: Speech start/end detection for natural conversation
- **Telephony-optimized**: 8kHz mu-law audio throughout

## Prerequisites

- Go 1.21+
- [Twilio account](https://www.twilio.com/) with a phone number
- [Deepgram API key](https://console.deepgram.com/)
- [ElevenLabs API key](https://elevenlabs.io/)
- [ngrok](https://ngrok.com/) or similar for local development

## Environment Variables

```bash
export ELEVENLABS_API_KEY="your-elevenlabs-api-key"
export DEEPGRAM_API_KEY="your-deepgram-api-key"
export TWILIO_ACCOUNT_SID="your-twilio-account-sid"
export TWILIO_AUTH_TOKEN="your-twilio-auth-token"
```

## Running Locally

1. **Start the server:**

   ```bash
   go run main.go
   ```

   The server starts on port 8080.

2. **Expose with ngrok:**

   ```bash
   ngrok http 8080
   ```

3. **Configure Twilio:**

   Set your Twilio phone number's webhook to:
   ```
   https://your-ngrok-url.ngrok.io/voice/inbound
   ```

4. **Call your Twilio number** and start talking!

## Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/voice/inbound` | POST | TwiML webhook for incoming calls |
| `/media-stream` | WebSocket | Twilio Media Streams connection |

## Customization

### Change the Voice

Edit the TTS pipeline configuration in `main.go`:

```go
ttsPipeline := pipeline.NewTTSPipeline(s.ttsProvider, pipeline.TTSPipelineConfig{
    VoiceID:      "Rachel",            // Change voice here
    OutputFormat: "ulaw",
    SampleRate:   8000,
    Model:        "eleven_turbo_v2_5",
})
```

### Add LLM Integration

Replace the `processUserInput` function with your LLM call:

```go
func processUserInput(input string) string {
    // Call Claude, GPT, or your preferred LLM here
    response, err := callLLM(input)
    if err != nil {
        return "I'm sorry, I encountered an error."
    }
    return response
}
```

## Dependencies

- [omnivoice](https://github.com/agentplexus/omnivoice) - Voice agent framework
- [omnivoice-deepgram](https://github.com/agentplexus/omnivoice-deepgram) - Deepgram STT provider
- [go-elevenlabs](https://github.com/agentplexus/go-elevenlabs) - ElevenLabs TTS SDK
- [omnivoice-twilio](https://github.com/agentplexus/omnivoice-twilio) - Twilio transport

## License

MIT
