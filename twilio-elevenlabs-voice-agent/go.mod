// Example: Voice agent using Twilio Media Streams with ElevenLabs TTS
//
// This example has its own go.mod to keep telephony provider dependencies
// separate from the main omnivoice module.
module github.com/agentplexus/omnivoice-examples/twilio-elevenlabs-voice-agent

go 1.23

require (
	github.com/agentplexus/go-elevenlabs v0.6.0
	github.com/agentplexus/omnivoice v0.1.0
	github.com/agentplexus/omnivoice-twilio v0.1.0
)
