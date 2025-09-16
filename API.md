# Voice Events API

The Voice Events API provides access to stored voice interaction events from the Loqa hub. All endpoints return JSON responses.

## Base URL

```
http://localhost:3000/api
```

## Endpoints

### List Voice Events

**GET** `/voice-events`

Retrieves a paginated list of voice events with optional filtering.

#### Query Parameters

| Parameter   | Type    | Default | Description                                    |
|-------------|---------|---------|------------------------------------------------|
| `page`      | integer | 1       | Page number (1-based)                        |
| `page_size` | integer | 20      | Number of events per page (max 100)          |
| `relay_id`   | string  | -       | Filter by specific relay ID                    |
| `intent`    | string  | -       | Filter by command intent                      |
| `success`   | boolean | -       | Filter by success status (true/false)        |
| `start_time`| string  | -       | Filter by start time (RFC3339 format)        |
| `end_time`  | string  | -       | Filter by end time (RFC3339 format)          |
| `sort_by`   | string  | timestamp | Sort field (`timestamp`, `confidence`, `processing_time`) |
| `sort_order`| string  | DESC    | Sort order (`ASC`, `DESC`)                    |

#### Example Request

```bash
curl "http://localhost:3000/api/voice-events?page=1&page_size=10&relay_id=kitchen-relay&intent=turn_on"
```

#### Example Response

```json
{
  "events": [
    {
      "uuid": "123e4567-e89b-12d3-a456-426614174000",
      "request_id": "kitchen-relay-001",
      "relay_id": "kitchen-relay",
      "timestamp": "2025-01-15T10:30:45Z",
      "audio_duration": 2.5,
      "sample_rate": 16000,
      "wake_word_detected": true,
      "transcription": "turn on the lights",
      "intent": "turn_on",
      "entities": {
        "device": "lights",
        "location": "kitchen"
      },
      "confidence": 0.95,
      "response_text": "Turning on the kitchen lights",
      "processing_time_ms": 150,
      "success": true,
      "error_message": null
    }
  ],
  "total": 1,
  "page": 1,
  "page_size": 10,
  "total_pages": 1
}
```

### Get Voice Event by ID

**GET** `/voice-events/{uuid}`

Retrieves a specific voice event by its UUID.

#### Path Parameters

| Parameter | Type   | Description           |
|-----------|--------|-----------------------|
| `uuid`    | string | Voice event UUID      |

#### Example Request

```bash
curl "http://localhost:3000/api/voice-events/123e4567-e89b-12d3-a456-426614174000"
```

#### Example Response

```json
{
  "uuid": "123e4567-e89b-12d3-a456-426614174000",
  "request_id": "kitchen-relay-001",
  "relay_id": "kitchen-relay",
  "timestamp": "2025-01-15T10:30:45Z",
  "audio_duration": 2.5,
  "sample_rate": 16000,
  "wake_word_detected": true,
  "transcription": "turn on the lights",
  "intent": "turn_on",
  "entities": {
    "device": "lights",
    "location": "kitchen"
  },
  "confidence": 0.95,
  "response_text": "Turning on the kitchen lights",
  "processing_time_ms": 150,
  "success": true,
  "error_message": null
}
```

### Create Voice Event

**POST** `/voice-events`

Creates a new voice event. Useful for testing or external integrations.

#### Request Body

```json
{
  "relay_id": "test-relay",
  "request_id": "test-request-001",
  "transcription": "hello loqa",
  "intent": "greeting",
  "entities": {
    "greeting_type": "hello"
  },
  "confidence": 0.88,
  "response_text": "Hello! How can I help you?",
  "audio_duration": 1.2,
  "sample_rate": 16000,
  "wake_word_detected": false
}
```

#### Example Request

```bash
curl -X POST "http://localhost:3000/api/voice-events" \
  -H "Content-Type: application/json" \
  -d '{
    "relay_id": "test-relay",
    "transcription": "hello loqa",
    "intent": "greeting",
    "confidence": 0.88,
    "response_text": "Hello! How can I help you?"
  }'
```

#### Example Response

```json
{
  "uuid": "456e7890-e12b-34d5-b678-789012345000",
  "request_id": "test-relay",
  "relay_id": "test-relay",
  "timestamp": "2025-01-15T10:35:00Z",
  "audio_hash": "sha256:def456...",
  "audio_duration": 1.2,
  "sample_rate": 16000,
  "wake_word_detected": false,
  "transcription": "hello loqa",
  "intent": "greeting",
  "entities": {
    "greeting_type": "hello"
  },
  "confidence": 0.88,
  "response_text": "Hello! How can I help you?",
  "processing_time_ms": 0,
  "success": true,
  "error_message": null
}
```

## Voice Event Schema

### VoiceEvent Object

| Field                | Type              | Description                                    |
|---------------------|-------------------|------------------------------------------------|
| `uuid`              | string            | Unique identifier for the event               |
| `request_id`        | string            | Request identifier from the relay              |
| `relay_id`           | string            | Identifier of the relay that sent the audio   |
| `timestamp`         | string (RFC3339)  | When the event was created                    |
| `audio_duration`    | number            | Duration of audio in seconds                  |
| `sample_rate`       | integer           | Audio sample rate (Hz)                        |
| `wake_word_detected`| boolean           | Whether wake word was detected                |
| `transcription`     | string            | Speech-to-text result                         |
| `intent`            | string            | Parsed command intent                         |
| `entities`          | object            | Extracted entities from the command           |
| `confidence`        | number (0-1)      | Confidence score for intent classification    |
| `response_text`     | string            | Response sent back to the relay                |
| `processing_time_ms`| integer           | Total processing time in milliseconds         |
| `success`           | boolean           | Whether the event was processed successfully  |
| `error_message`     | string (nullable) | Error message if processing failed            |

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

## STT Confidence & Wake Word Processing

The Loqa hub includes advanced speech-to-text processing with confidence thresholds and wake word normalization.

### Wake Word Detection

The system automatically detects and strips wake words from transcriptions before intent parsing:

#### Supported Wake Word Patterns
- `"Hey Loqa"` (primary)
- `"Hey Luca"`, `"Hey Luka"`, `"Hey Loca"` (common STT misrecognitions)
- `"Hey Logic"`, `"Hey Local"` (other variants)
- `"Loqa"`, `"Luca"`, `"Luka"` (standalone variants)

#### Processing Behavior
- **Case insensitive**: `"HEY LOQA"` is handled the same as `"hey loqa"`
- **Punctuation tolerant**: `"Hey Loqa, turn on lights"` → `"turn on lights"`
- **Preserves original**: Original transcription is logged for debugging

### Confidence Thresholds

The system estimates transcription confidence and handles low-confidence cases gracefully:

#### Confidence Estimation Factors
- **Text length**: Very short utterances (< 3 chars) reduce confidence
- **Pattern recognition**: Nonsensical patterns (`"???"`, `"..."`) reduce confidence
- **Repetition detection**: Stammering patterns (`"aaaaaah"`) reduce confidence
- **Wake word boost**: Presence of wake words increases confidence

#### Confidence Handling
- **High confidence (≥60%)**: Process normally
- **Low confidence (<60%)**: Send confirmation prompt
- **Empty after wake word stripping**: Always request confirmation

#### Example Confidence Responses
```json
{
  "transcription": "turn lights",
  "confidence": 0.45,
  "response_text": "I'm not sure I heard you correctly. Did you say 'turn lights'? Please repeat if that's not right.",
  "command": "confirmation_needed"
}
```

### Enhanced Logging

Voice events now include detailed STT processing information:

```json
{
  "uuid": "123e4567-e89b-12d3-a456-426614174000",
  "transcription": "turn on the lights",
  "confidence": 0.85,
  "wake_word_detected": true,
  "wake_word_variant": "hey loqa",
  "original_transcription": "Hey Loqa turn on the lights",
  "needs_confirmation": false
}
```

### Integration Notes

- **Backward compatible**: Existing `Transcribe()` method still works
- **Enhanced method**: New `TranscribeWithConfidence()` provides detailed results
- **Graceful fallback**: Low confidence gracefully handled without errors
- **User experience**: Confirmation prompts improve interaction reliability