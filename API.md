# Loqa Hub API

The Loqa Hub API provides HTTP/1.1 streaming transport and RESTful system management endpoints. The new stateless architecture focuses on real-time processing without persistent storage.

## Base URL

```
http://localhost:3000
```

## API Categories

- [Streaming Transport API](#streaming-transport-api) - HTTP/1.1 binary streaming for ESP32 pucks
- [System Information API](#system-information-api) - Health, capabilities, and performance tiers
- [Arbitration API](#arbitration-api) - Wake word arbitration statistics
- [Intent Processing API](#intent-processing-api) - Text-based intent cascade testing

---

## Streaming Transport API

The streaming transport API provides HTTP/1.1 chunked transfer encoding for real-time communication with ESP32 pucks.

### Binary Frame Streaming

**POST** `/stream/puck`

Establishes an HTTP/1.1 streaming connection for binary frame communication with ESP32 devices.

#### Headers

| Header | Value | Description |
|--------|-------|-------------|
| `Content-Type` | `application/octet-stream` | Binary frame data |
| `Transfer-Encoding` | `chunked` | HTTP/1.1 chunked encoding |
| `Connection` | `keep-alive` | Persistent connection |

#### Binary Frame Format

Each frame consists of a 24-byte header followed by optional data payload:

```
+------------------+------------------+------------------+
| Magic (4 bytes)  | Type (1 byte)    | Reserved (1 byte)|
+------------------+------------------+------------------+
| Length (2 bytes) | SessionID (4 bytes)                 |
+------------------+----------------------------------+
| Sequence (4 bytes)                                   |
+------------------------------------------------------+
| Timestamp (8 bytes)                                  |
+------------------------------------------------------+
| Data payload (0-4072 bytes)                         |
+------------------------------------------------------+
```

#### Frame Types

| Type | Value | Description |
|------|-------|-------------|
| `FrameTypeAudioData` | `0x01` | Audio PCM data |
| `FrameTypeAudioEnd` | `0x02` | End of audio stream |
| `FrameTypeWakeWord` | `0x03` | Wake word detection |
| `FrameTypeHeartbeat` | `0x10` | Connection keepalive |
| `FrameTypeHandshake` | `0x11` | Session initialization |
| `FrameTypeError` | `0x12` | Error notification |
| `FrameTypeArbitration` | `0x13` | Wake word arbitration |
| `FrameTypeResponse` | `0x20` | Hub response |
| `FrameTypeStatus` | `0x21` | Status update |

#### Example Usage

```bash
# ESP32 establishes streaming connection
curl -X POST "http://localhost:3000/stream/puck" \
  -H "Content-Type: application/octet-stream" \
  -H "Transfer-Encoding: chunked" \
  --data-binary @audio_frames.bin
```

---

## System Information API

### Health Status

**GET** `/health`

Returns the current health status of the Hub and all connected services.

#### Example Request

```bash
curl "http://localhost:3000/health"
```

#### Example Response

```json
{
  "status": "ok",
  "timestamp": "2025-01-15T10:30:45Z",
  "architecture": "HTTP/1.1 Binary Streaming",
  "tier": "standard",
  "services": {
    "stt_available": true,
    "tts_available": true,
    "llm_available": true,
    "nats_available": true
  },
  "degraded": false
}
```

### System Capabilities

**GET** `/api/capabilities`

Returns detailed system capabilities including hardware information and performance metrics.

#### Example Request

```bash
curl "http://localhost:3000/api/capabilities"
```

#### Example Response

```json
{
  "tier": "standard",
  "services": {
    "stt_available": true,
    "tts_available": true,
    "llm_available": true,
    "nats_available": true
  },
  "hardware": {
    "cpu_cores": 8,
    "memory_gb": 16,
    "architecture": "amd64",
    "os": "linux"
  },
  "performance": {
    "avg_cpu_usage": 45.2,
    "avg_memory_usage": 32.1,
    "stt_latency": "150ms",
    "tts_latency": "100ms",
    "llm_latency": "800ms",
    "last_measured": "2025-01-15T10:30:00Z"
  },
  "last_detected": "2025-01-15T10:30:45Z",
  "degraded": false
}
```

### Performance Tier Information

**GET** `/api/tier`

Returns current performance tier and available features.

#### Example Request

```bash
curl "http://localhost:3000/api/tier"
```

#### Example Response

```json
{
  "current_tier": "standard",
  "features": {
    "local_llm": true,
    "streaming_responses": false,
    "reflex_only": true
  }
}
```

### Performance Tiers

| Tier | Requirements | Features |
|------|-------------|----------|
| **Basic** | 2+ cores, 2GB+ RAM | Reflex-only processing |
| **Standard** | 4+ cores, 8GB+ RAM | Local LLM, full features |
| **Pro** | 8+ cores, 16GB+ RAM | Advanced processing, experimental features |

---

## Arbitration API

### Wake Word Arbitration Statistics

**GET** `/api/arbitration/stats`

Returns statistics about wake word arbitration including recent competitions and performance metrics.

#### Example Request

```bash
curl "http://localhost:3000/api/arbitration/stats"
```

#### Example Response

```json
{
  "total_arbitrations": 1247,
  "successful_arbitrations": 1198,
  "failed_arbitrations": 49,
  "average_arbitration_time": "85ms",
  "recent_arbitrations": [
    {
      "timestamp": "2025-01-15T10:30:45Z",
      "winner_puck_id": "kitchen-puck-01",
      "winner_score": 0.92,
      "total_detections": 3,
      "arbitration_time": "78ms",
      "all_detections": [
        {
          "puck_id": "kitchen-puck-01",
          "confidence": 0.95,
          "distance": 1.2,
          "signal_quality": 0.89,
          "score": 0.92
        },
        {
          "puck_id": "living-room-puck-02",
          "confidence": 0.87,
          "distance": 3.1,
          "signal_quality": 0.76,
          "score": 0.71
        }
      ]
    }
  ],
  "performance_metrics": {
    "average_competition_size": 2.3,
    "most_active_puck": "kitchen-puck-01",
    "fairness_score": 0.88
  }
}
```

---

## Intent Processing API

### Process Text Intent

**POST** `/api/intent/process`

Processes text through the intent cascade for testing and development purposes.

#### Request Body

```json
{
  "text": "turn on the living room lights"
}
```

#### Example Request

```bash
curl -X POST "http://localhost:3000/api/intent/process" \
  -H "Content-Type: application/json" \
  -d '{"text": "turn on the living room lights"}'
```

#### Example Response

```json
{
  "type": "device_control",
  "confidence": 0.95,
  "text": "turn on the living room lights",
  "entities": {
    "device": "lights",
    "location": "living room",
    "action": "turn_on"
  },
  "response": "Turning on the living room lights",
  "processing_tier": "local_llm",
  "processing_time_ms": 245
}
```

## Error Responses

All endpoints may return these error responses:

### 400 Bad Request
```json
{
  "error": "Invalid request parameters"
}
```

### 404 Not Found
```json
{
  "error": "Voice event not found"
}
```

### 405 Method Not Allowed
```json
{
  "error": "Method not allowed"
}
```

### 500 Internal Server Error
```json
{
  "error": "Internal server error"
}
```

## Rate Limiting

Currently no rate limiting is implemented. In production, consider implementing rate limiting to prevent abuse.

## Authentication

Currently no authentication is required. In production environments, consider implementing API key or OAuth-based authentication.

## CORS

CORS headers are not currently set. For web-based frontends, consider adding appropriate CORS configuration.

---

## Integration Notes

### HTTP/1.1 Streaming Transport
- **Real-time communication**: Uses chunked transfer encoding for efficient streaming
- **ESP32 optimized**: 4KB frame size limit for embedded device compatibility
- **Stateless sessions**: No persistent storage, session state maintained in frame headers
- **Binary efficiency**: Minimal overhead with 24-byte frame headers

### System Monitoring
- **Health endpoints**: Use `/health` and `/api/capabilities` for service monitoring
- **Performance tiers**: Automatic adaptation based on system capabilities
- **Graceful degradation**: System automatically falls back to basic tier during issues
- **Wake word arbitration**: Fair competition prevents single device dominance

### Development and Testing
- **Intent processing**: Use `/api/intent/process` for testing text-based intent cascade
- **Arbitration stats**: Monitor wake word competition fairness and performance
- **Tier information**: Understand current system capabilities and feature availability