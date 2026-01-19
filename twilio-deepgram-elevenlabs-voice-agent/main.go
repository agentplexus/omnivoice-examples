// Example: Full voice agent using Deepgram STT and ElevenLabs TTS
//
// This example demonstrates a complete voice agent pipeline:
//   - Twilio Media Streams for telephony transport (mu-law audio)
//   - Deepgram streaming STT for speech-to-text
//   - ElevenLabs WebSocket TTS for text-to-speech
//
// Architecture:
//
//	┌──────────┐        ┌─────────────────┐        ┌───────────────────────────────┐
//	│  Caller  │◄──────►│     Twilio      │◄──────►│         OmniVoice             │
//	│  (PSTN)  │  PSTN  │   Media         │WebSocket│                               │
//	└──────────┘        │   Streams       │ (μ-law) │  ┌─────────┐     ┌─────────┐  │
//	                    └─────────────────┘         │  │Deepgram │     │ElevenLabs│ │
//	                                                │  │  STT    │────►│   TTS   │  │
//	                                                │  └────┬────┘     └────┬────┘  │
//	                                                │       │               │       │
//	                                                │       ▼               │       │
//	                                                │  ┌─────────────────┐  │       │
//	                                                │  │   Agent Logic   │──┘       │
//	                                                │  │  (echo/LLM)     │          │
//	                                                │  └─────────────────┘          │
//	                                                └───────────────────────────────┘
//
// Flow:
//  1. Caller dials Twilio phone number
//  2. Twilio connects via Media Streams (mu-law audio)
//  3. Audio goes to Deepgram STT → transcripts
//  4. Transcripts are processed (echo/LLM)
//  5. Response goes to ElevenLabs TTS → audio
//  6. Audio (ulaw) streams back to caller via Twilio
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	elevenlabs "github.com/agentplexus/go-elevenlabs"
	elevenvoice "github.com/agentplexus/go-elevenlabs/omnivoice/tts"
	deepgramstt "github.com/agentplexus/omnivoice-deepgram/omnivoice/stt"
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

	deepgramAPIKey := os.Getenv("DEEPGRAM_API_KEY")
	if deepgramAPIKey == "" {
		log.Fatal("DEEPGRAM_API_KEY environment variable required")
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

	// Create Deepgram STT provider
	sttProvider, err := deepgramstt.New(deepgramstt.WithAPIKey(deepgramAPIKey))
	if err != nil {
		log.Fatalf("Failed to create Deepgram provider: %v", err)
	}

	// Create Twilio Media Streams transport
	twilioTransport, err := twiliotransport.New(
		twiliotransport.WithAccountSID(twilioAccountSID),
		twiliotransport.WithAuthToken(twilioAuthToken),
	)
	if err != nil {
		log.Fatalf("Failed to create Twilio transport: %v", err)
	}
	defer func() { _ = twilioTransport.Close() }()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create server with providers
	server := &Server{
		ttsProvider:     ttsProvider,
		sttProvider:     sttProvider,
		twilioTransport: twilioTransport,
	}

	// Start HTTP server
	http.HandleFunc("/voice/inbound", server.handleInboundCall)
	http.HandleFunc("/media-stream", server.handleMediaStream)

	addr := ":8080"
	log.Printf("Starting voice agent server on %s", addr)

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
	sttProvider     *deepgramstt.Provider
	twilioTransport *twiliotransport.Provider
}

// handleInboundCall returns TwiML to connect the call to Media Streams.
func (s *Server) handleInboundCall(w http.ResponseWriter, r *http.Request) {
	from := r.FormValue("From")
	to := r.FormValue("To")
	callSID := r.FormValue("CallSid")

	log.Printf("Incoming call: %s -> %s (SID: %s)", from, to, callSID)

	// Return TwiML to connect to Media Streams
	wsURL := fmt.Sprintf("wss://%s/media-stream", r.Host)

	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
    <Say>Connecting you to the voice assistant.</Say>
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

// handleSession manages a single voice session with full STT → Agent → TTS flow.
func (s *Server) handleSession(ctx context.Context, conn transport.Connection) {
	sessionID := conn.ID()
	log.Printf("New session: %s", sessionID)

	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	// Create TTS pipeline configured for telephony
	ttsPipeline := pipeline.NewTTSPipeline(s.ttsProvider, pipeline.TTSPipelineConfig{
		VoiceID:      "Rachel",
		OutputFormat: "ulaw",
		SampleRate:   8000,
		Model:        "eleven_turbo_v2_5",
		OnError: func(err error) {
			slog.Error("TTS error", "error", err, "session", sessionID)
		},
		OnComplete: func() {
			slog.Debug("TTS complete", "session", sessionID)
		},
	})

	// Track pending transcript for forming complete utterances
	var pendingTranscript strings.Builder
	var transcriptMu sync.Mutex

	// Create STT pipeline configured for telephony
	sttConfig := pipeline.STTPipelineConfig{
		Model:      "nova-2",
		Language:   "en-US",
		Encoding:   "mulaw",
		SampleRate: 8000,
		Channels:   1,

		OnTranscript: func(transcript string, isFinal bool) {
			transcriptMu.Lock()
			defer transcriptMu.Unlock()

			if isFinal {
				// Append final transcript and process complete utterance
				pendingTranscript.WriteString(transcript)
				fullText := strings.TrimSpace(pendingTranscript.String())
				pendingTranscript.Reset()

				if fullText != "" {
					log.Printf("[%s] User said: %s", sessionID, fullText)

					// Process the transcript and generate response
					// For this demo, we echo back what the user said
					// In production, you would send this to an LLM (Claude, GPT, etc.)
					response := processUserInput(fullText)

					// Send response to TTS pipeline
					if err := ttsPipeline.SynthesizeToConnection(sessionCtx, response, conn); err != nil {
						slog.Error("failed to synthesize response", "error", err, "session", sessionID)
					}
				}
			} else {
				// Accumulate interim results for context
				slog.Debug("interim transcript", "text", transcript, "session", sessionID)
			}
		},

		OnSpeechStart: func() {
			log.Printf("[%s] Speech started", sessionID)
			// Optionally stop TTS when user starts speaking (barge-in)
			if ttsPipeline.IsActive() {
				ttsPipeline.Stop()
			}
		},

		OnSpeechEnd: func() {
			log.Printf("[%s] Speech ended", sessionID)
		},

		OnError: func(err error) {
			slog.Error("STT error", "error", err, "session", sessionID)
		},
	}

	sttPipeline := pipeline.NewSTTPipeline(s.sttProvider, sttConfig)

	// Start STT pipeline
	if err := sttPipeline.StartFromConnection(sessionCtx, conn); err != nil {
		slog.Error("failed to start STT pipeline", "error", err)
		_ = conn.Close()
		return
	}

	// Send initial greeting
	greeting := "Hello! I'm your voice assistant powered by Deepgram and ElevenLabs. How can I help you today?"
	if err := ttsPipeline.SynthesizeToConnection(sessionCtx, greeting, conn); err != nil {
		slog.Error("failed to send greeting", "error", err, "session", sessionID)
	}

	// Keep session alive until context is cancelled or connection closes
	select {
	case <-sessionCtx.Done():
	case event := <-conn.Events():
		if event.Type == transport.EventDisconnected {
			log.Printf("[%s] Connection closed", sessionID)
		}
	}

	// Cleanup
	sttPipeline.Stop()
	ttsPipeline.Stop()
	_ = conn.Close()
	log.Printf("Session ended: %s", sessionID)
}

// processUserInput processes user speech and returns a response.
// In production, this would call an LLM like Claude or GPT.
func processUserInput(input string) string {
	input = strings.ToLower(input)

	// Simple echo bot with a few canned responses
	switch {
	case strings.Contains(input, "hello") || strings.Contains(input, "hi"):
		return "Hello! It's nice to hear from you. What would you like to talk about?"

	case strings.Contains(input, "how are you"):
		return "I'm doing great, thank you for asking! I'm here and ready to help you with anything you need."

	case strings.Contains(input, "goodbye") || strings.Contains(input, "bye"):
		return "Goodbye! It was nice talking with you. Have a wonderful day!"

	case strings.Contains(input, "help"):
		return "I can help you with various tasks. Just tell me what you need, and I'll do my best to assist you."

	case strings.Contains(input, "weather"):
		return "I don't have access to real-time weather data, but you could try asking a weather service for accurate forecasts."

	case strings.Contains(input, "time"):
		return fmt.Sprintf("The current time is %s.", time.Now().Format("3:04 PM"))

	default:
		// Echo back with acknowledgment
		return fmt.Sprintf("I heard you say: %s. Is there anything specific you'd like me to help you with?", input)
	}
}
