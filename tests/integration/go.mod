module github.com/loqa-voice-assistant/tests/integration

go 1.23.0

require (
	github.com/loqa-voice-assistant/proto/go v0.0.0
	google.golang.org/grpc v1.58.3
)

replace github.com/loqa-voice-assistant/proto/go => github.com/loqalabs/loqa-proto/go latest
