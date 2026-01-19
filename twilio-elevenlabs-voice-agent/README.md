# Twilio + ElevenLabs Voice Agent

A voice agent example using Twilio Media Streams for telephony transport and ElevenLabs for text-to-speech synthesis.

## Architecture

```
┌──────────┐        ┌─────────────────┐         ┌──────────────────────────────┐
│  Caller  │◄──────►│      Twilio     │◄───────►│         OmniVoice            │
│  (PSTN)  │  PSTN  │      Media      │WebSocket│                              │
└──────────┘        │      Streams    │ (μ-law) │  ┌─────┐           ┌─────┐   │
                    └─────────────────┘         │  │ STT │◄──────────│ TTS │   │
                                                │  └──┬──┘  (text)   └──┬──┘   │
                                                │     │                 │      │
                                                │     ▼                 │      │
                                                │  ┌─────────────────┐  │      │
                                                │  │       LLM       │──┘      │
                                                │  │    (Claude)     │         │
                                                │  └─────────────────┘         │
                                                └──────────────────────────────┘
```

## Key Features

- **Native telephony audio**: ElevenLabs outputs `ulaw_8000` directly - no audio conversion needed for Twilio
- **Low latency**: Uses `eleven_turbo_v2_5` model optimized for real-time synthesis
- **WebSocket streaming**: Real-time audio streaming to/from Twilio Media Streams

## Prerequisites

- Go 1.23+
- ElevenLabs API key
- Twilio account with:
  - Account SID
  - Auth Token
  - A phone number configured for voice

## Environment Variables

```bash
export ELEVENLABS_API_KEY="your-elevenlabs-api-key"
export TWILIO_ACCOUNT_SID="your-twilio-account-sid"
export TWILIO_AUTH_TOKEN="your-twilio-auth-token"
```

## Running

```bash
go run .
```

The server listens on `:8080` with two endpoints:

- `/voice/inbound` - TwiML webhook for incoming calls
- `/media-stream` - WebSocket endpoint for Twilio Media Streams

## Twilio Configuration

Configure your Twilio phone number's voice webhook to point to:

```
https://your-domain.com/voice/inbound
```

Use ngrok or similar for local development:

```bash
ngrok http 8080
```

## Dependencies

This example depends on:

- [go-elevenlabs](https://github.com/agentplexus/go-elevenlabs) - ElevenLabs Go client
- [omnivoice](https://github.com/agentplexus/omnivoice) - Voice pipeline framework
- [omnivoice-twilio](https://github.com/agentplexus/omnivoice-twilio) - Twilio Media Streams transport
