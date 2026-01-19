// Example: Voice agent using Twilio Media Streams with ElevenLabs TTS
//
// NOTE: This example may be moved to a separate integration examples repo
// to avoid go-elevenlabs depending on the Twilio SDK.
//
// This example demonstrates how to build a voice agent using:
// - Twilio Media Streams for telephony transport (mu-law audio)
// - ElevenLabs WebSocket TTS for voice synthesis (native ulaw_8000 output)
//
// Architecture (Option B from omnivoice TRD):
//
//	┌──────────┐        ┌─────────────────┐        ┌───────────────────────────────┐
//	│  Caller  │◄──────►│     Twilio      │◄──────►│         OmniVoice             │
//	│  (PSTN)  │  PSTN  │   Media         │WebSocket│                               │
//	└──────────┘        │   Streams       │ (μ-law) │  ┌─────┐           ┌─────┐   │
//	                    └─────────────────┘         │  │ STT │◄──────────│ TTS │   │
//	                                                │  └──┬──┘  (text)  └──┬──┘   │
//	                                                │     │                 │      │
//	                                                │     ▼                 │      │
//	                                                │  ┌─────────────────┐  │      │
//	                                                │  │       LLM       │──┘      │
//	                                                │  │    (Claude)     │         │
//	                                                │  └─────────────────┘         │
//	                                                └───────────────────────────────┘
//
// Key feature: ElevenLabs supports native ulaw_8000 output, so no audio conversion
// is needed for the outbound (TTS → Twilio) path.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	elevenlabs "github.com/agentplexus/go-elevenlabs"
	elevenvoice "github.com/agentplexus/go-elevenlabs/omnivoice/tts"
	twiliotransport "github.com/agentplexus/omnivoice-twilio/transport"
	"github.com/agentplexus/omnivoice/pipeline"
	"github.com/agentplexus/omnivoice/transport"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get API keys from environment
	elevenLabsAPIKey := os.Getenv("ELEVENLABS_API_KEY")
	if elevenLabsAPIKey == "" {
		log.Fatal("ELEVENLABS_API_KEY environment variable required")
	}

	twilioAccountSID := os.Getenv("TWILIO_ACCOUNT_SID")
	twilioAuthToken := os.Getenv("TWILIO_AUTH_TOKEN")
	if twilioAccountSID == "" || twilioAuthToken == "" {
		log.Fatal("TWILIO_ACCOUNT_SID and TWILIO_AUTH_TOKEN environment variables required")
	}

	// Create ElevenLabs TTS provider
	elevenClient, err := elevenlabs.NewClient(elevenlabs.WithAPIKey(elevenLabsAPIKey))
	if err != nil {
		log.Fatalf("Failed to create ElevenLabs client: %v", err)
	}
	ttsProvider := elevenvoice.NewWithClient(elevenClient)

	// Create Twilio Media Streams transport
	twilioTransport, err := twiliotransport.New(
		twiliotransport.WithAccountSID(twilioAccountSID),
		twiliotransport.WithAuthToken(twilioAuthToken),
	)
	if err != nil {
		log.Fatalf("Failed to create Twilio transport: %v", err)
	}
	defer func() {
		if err := twilioTransport.Close(); err != nil {
			slog.Error("failed to close Twilio transport", "error", err)
		}
	}()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create server with handlers
	server := &Server{
		ttsProvider:     ttsProvider,
		twilioTransport: twilioTransport,
	}

	// Start HTTP server
	http.HandleFunc("/voice/inbound", server.handleInboundCall)
	http.HandleFunc("/media-stream", server.handleMediaStream)

	addr := ":8080"
	log.Printf("Starting server on %s", addr)

	httpServer := &http.Server{
		Addr:              addr,
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start listening for Media Streams connections
	connCh, err := twilioTransport.Listen(ctx, "/media-stream")
	if err != nil {
		log.Fatalf("Failed to start Media Streams listener: %v", err)
	}

	// Handle incoming connections
	go server.handleConnections(ctx, connCh)

	go func() {
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down...")
	_ = httpServer.Close()
}

// Server handles voice agent connections.
type Server struct {
	ttsProvider     *elevenvoice.Provider
	twilioTransport *twiliotransport.Provider
}

// handleInboundCall returns TwiML to connect the call to Media Streams.
func (s *Server) handleInboundCall(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("From")
	to := r.FormValue("To")
	callSID := r.FormValue("CallSid")

	log.Printf("Incoming call: %s -> %s (SID: %s)", from, to, callSID)

	// Return TwiML to connect to Media Streams
	// Note: Using <Stream> for raw audio, not <ConversationRelay>
	wsURL := fmt.Sprintf("wss://%s/media-stream", r.Host)

	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Say>Hello, connecting you to our AI assistant.</Say>
    <Connect>
        <Stream url="%s">
            <Parameter name="callSid" value="%s"/>
            <Parameter name="caller" value="%s"/>
        </Stream>
    </Connect>
</Response>`, wsURL, callSID, from)

	w.Header().Set("Content-Type", "application/xml")
	if _, err := w.Write([]byte(twiml)); err != nil {
		slog.Error("failed to write TwiML", "error", err)
	}
}

// handleMediaStream upgrades HTTP to WebSocket and handles Media Streams.
func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	if err := s.twilioTransport.HandleWebSocket(w, r, "/media-stream"); err != nil {
		slog.Error("WebSocket handling failed", "error", err)
	}
}

// handleConnections processes incoming Media Streams connections.
func (s *Server) handleConnections(ctx context.Context, connCh <-chan transport.Connection) {
	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-connCh:
			go s.handleSession(ctx, conn)
		}
	}
}

// handleSession manages a single voice session.
func (s *Server) handleSession(ctx context.Context, conn transport.Connection) {
	log.Printf("New session: %s", conn.ID())

	// Create TTS pipeline configured for telephony
	// Using "ulaw" format so ElevenLabs outputs mu-law directly - no conversion needed!
	ttsConfig := pipeline.TTSPipelineConfig{
		VoiceID:      "Rachel",            // ElevenLabs voice
		OutputFormat: "ulaw",              // Native mu-law output for Twilio
		SampleRate:   8000,                // Telephony sample rate
		Model:        "eleven_turbo_v2_5", // Low-latency model
		OnError: func(err error) {
			slog.Error("TTS error", "error", err, "session", conn.ID())
		},
		OnComplete: func() {
			slog.Info("TTS complete", "session", conn.ID())
		},
	}

	ttsPipeline := pipeline.NewTTSPipeline(s.ttsProvider, ttsConfig)

	// Synthesize a greeting
	// In a real agent, this would be triggered by STT transcripts + LLM responses
	err := ttsPipeline.SynthesizeToConnection(ctx, "Hello! How can I help you today?", conn)
	if err != nil {
		slog.Error("TTS synthesis failed", "error", err)
	}

	// TODO: Implement full STT → LLM → TTS loop
	// 1. Read audio from conn.AudioOut() (mu-law from caller)
	// 2. Convert mu-law to PCM using omnivoice/audio/codec
	// 3. Send PCM to Deepgram STT for transcription
	// 4. Send transcript to Claude LLM
	// 5. Send LLM response to ElevenLabs TTS (via pipeline)
	// 6. TTS audio (ulaw) goes directly to Twilio via pipeline

	// Keep session alive until context is cancelled
	<-ctx.Done()
	_ = conn.Close()
	log.Printf("Session ended: %s", conn.ID())
}
