# OmniVoice Examples

Integration examples for [OmniVoice](https://github.com/agentplexus/omnivoice) voice pipelines.

These examples are maintained in a separate repository to avoid adding heavy telephony/provider dependencies to the core OmniVoice module.

## Examples

| Example | Description |
|---------|-------------|
| [twilio-elevenlabs-voice-agent](./twilio-elevenlabs-voice-agent) | Voice agent using Twilio Media Streams + ElevenLabs TTS |

## Structure

Each example is a standalone Go module with its own `go.mod` file. This allows each example to have different dependencies without affecting others.

## Running Examples

```bash
cd <example-directory>
go run .
```

See each example's README for specific requirements and configuration.

## Related Projects

- [omnivoice](https://github.com/agentplexus/omnivoice) - Voice pipeline framework
- [omnivoice-twilio](https://github.com/agentplexus/omnivoice-twilio) - Twilio Media Streams transport
- [go-elevenlabs](https://github.com/agentplexus/go-elevenlabs) - ElevenLabs Go client
