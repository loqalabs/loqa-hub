/*
 * This file is part of Loqa (https://github.com/loqalabs/loqa).
 * Copyright (C) 2025 Loqa Labs
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program. If not, see <https://www.gnu.org/licenses/>.
 */

package llm

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"
)

// StreamingAudioPipeline manages progressive TTS synthesis from streaming text
type StreamingAudioPipeline struct {
	ttsClient      TextToSpeech
	ttsOptions     *TTSOptions
	maxConcurrent  int
	mu             sync.RWMutex
	activeContexts map[string]*PipelineContext
}

// PipelineContext manages a single streaming audio session
type PipelineContext struct {
	ID              string
	AudioChunks     chan *AudioChunk
	ErrorChan       chan error
	Cancel          context.CancelFunc
	Metrics         *PipelineMetrics
	synthesisQueue  chan *SynthesisJob
	completedJobs   chan *SynthesisJob
	wg              sync.WaitGroup
	mu              sync.RWMutex
}

// AudioChunk represents a synthesized audio segment
type AudioChunk struct {
	Audio       io.Reader
	ContentType string
	Length      int64
	SequenceID  int
	IsLast      bool
	Phrase      string
	Timestamp   time.Time
}

// SynthesisJob represents a TTS synthesis task
type SynthesisJob struct {
	ID          int
	Phrase      string
	Options     *TTSOptions
	StartTime   time.Time
	CompletedAt time.Time
	Result      *TTSResult
	Error       error
}

// PipelineMetrics tracks audio pipeline performance
type PipelineMetrics struct {
	StartTime           time.Time
	FirstAudioTime      time.Time
	TotalPhrases        int
	SynthesizedPhrases  int
	FailedSynthesis     int
	AverageSynthesisTime time.Duration
	TotalAudioDuration  time.Duration
	QueueHighWaterMark  int
}

// NewStreamingAudioPipeline creates a new streaming audio pipeline
func NewStreamingAudioPipeline(ttsClient TextToSpeech, options *TTSOptions) *StreamingAudioPipeline {
	if options == nil {
		options = &TTSOptions{
			Voice:          "af_bella",
			Speed:          1.0,
			ResponseFormat: "mp3",
			Normalize:      true,
		}
	}

	return &StreamingAudioPipeline{
		ttsClient:      ttsClient,
		ttsOptions:     options,
		maxConcurrent:  3, // Parallel TTS synthesis
		activeContexts: make(map[string]*PipelineContext),
	}
}

// StartPipeline creates a new streaming audio context
func (sap *StreamingAudioPipeline) StartPipeline(contextID string, phraseStream <-chan string) (*PipelineContext, error) {
	sap.mu.Lock()
	defer sap.mu.Unlock()

	// Check if context already exists
	if _, exists := sap.activeContexts[contextID]; exists {
		return nil, fmt.Errorf("pipeline context %s already exists", contextID)
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	// Create pipeline context
	pipelineCtx := &PipelineContext{
		ID:             contextID,
		AudioChunks:    make(chan *AudioChunk, 10),
		ErrorChan:      make(chan error, 5),
		Cancel:         cancel,
		Metrics:        &PipelineMetrics{StartTime: time.Now()},
		synthesisQueue: make(chan *SynthesisJob, 20),
		completedJobs:  make(chan *SynthesisJob, 20),
	}

	// Store context
	sap.activeContexts[contextID] = pipelineCtx

	// Start pipeline workers
	go sap.phraseProcessor(ctx, pipelineCtx, phraseStream)
	go sap.synthesisWorkerPool(ctx, pipelineCtx)
	go sap.audioSequencer(ctx, pipelineCtx)

	log.Printf("🎵 Started streaming audio pipeline for context: %s", contextID)
	return pipelineCtx, nil
}

// StopPipeline stops and cleans up a streaming audio context
func (sap *StreamingAudioPipeline) StopPipeline(contextID string) {
	sap.mu.Lock()
	defer sap.mu.Unlock()

	pipelineCtx, exists := sap.activeContexts[contextID]
	if !exists {
		return
	}

	// Cancel context
	pipelineCtx.Cancel()

	// Wait for workers to finish
	pipelineCtx.wg.Wait()

	// Close channels
	close(pipelineCtx.AudioChunks)
	close(pipelineCtx.ErrorChan)

	// Remove from active contexts
	delete(sap.activeContexts, contextID)

	log.Printf("🛑 Stopped streaming audio pipeline for context: %s", contextID)
}

// phraseProcessor receives phrases and queues them for synthesis
func (sap *StreamingAudioPipeline) phraseProcessor(ctx context.Context, pipelineCtx *PipelineContext, phraseStream <-chan string) {
	pipelineCtx.wg.Add(1)
	defer pipelineCtx.wg.Done()
	defer close(pipelineCtx.synthesisQueue)

	sequenceID := 0

	for {
		select {
		case <-ctx.Done():
			return

		case phrase, ok := <-phraseStream:
			if !ok {
				// Phrase stream closed, send final job
				finalJob := &SynthesisJob{
					ID:        sequenceID,
					Phrase:    "", // Empty phrase signals end
					Options:   sap.ttsOptions,
					StartTime: time.Now(),
				}

				select {
				case pipelineCtx.synthesisQueue <- finalJob:
				case <-ctx.Done():
				}
				return
			}

			if phrase == "" {
				continue
			}

			pipelineCtx.Metrics.TotalPhrases++

			// Create synthesis job
			job := &SynthesisJob{
				ID:        sequenceID,
				Phrase:    phrase,
				Options:   sap.ttsOptions,
				StartTime: time.Now(),
			}

			// Update queue high water mark
			queueLen := len(pipelineCtx.synthesisQueue)
			if queueLen > pipelineCtx.Metrics.QueueHighWaterMark {
				pipelineCtx.Metrics.QueueHighWaterMark = queueLen
			}

			select {
			case pipelineCtx.synthesisQueue <- job:
				sequenceID++
			case <-ctx.Done():
				return
			}
		}
	}
}

// synthesisWorkerPool runs parallel TTS synthesis workers
func (sap *StreamingAudioPipeline) synthesisWorkerPool(ctx context.Context, pipelineCtx *PipelineContext) {
	pipelineCtx.wg.Add(1)
	defer pipelineCtx.wg.Done()
	defer close(pipelineCtx.completedJobs)

	// Start worker goroutines
	var workerWg sync.WaitGroup
	for i := 0; i < sap.maxConcurrent; i++ {
		workerWg.Add(1)
		go sap.synthesisWorker(ctx, pipelineCtx, &workerWg)
	}

	// Wait for all workers to complete
	workerWg.Wait()
}

// synthesisWorker processes individual TTS synthesis jobs
func (sap *StreamingAudioPipeline) synthesisWorker(ctx context.Context, pipelineCtx *PipelineContext, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case job, ok := <-pipelineCtx.synthesisQueue:
			if !ok {
				return
			}

			// Handle end-of-stream marker
			if job.Phrase == "" {
				job.CompletedAt = time.Now()
				select {
				case pipelineCtx.completedJobs <- job:
				case <-ctx.Done():
				}
				return
			}

			// Perform TTS synthesis
			result, err := sap.ttsClient.Synthesize(job.Phrase, job.Options)
			job.CompletedAt = time.Now()

			if err != nil {
				job.Error = err
				pipelineCtx.Metrics.FailedSynthesis++

				select {
				case pipelineCtx.ErrorChan <- fmt.Errorf("TTS synthesis failed for job %d: %w", job.ID, err):
				case <-ctx.Done():
					return
				}
			} else {
				job.Result = result
				pipelineCtx.Metrics.SynthesizedPhrases++

				// Update synthesis time metrics
				synthesisTime := job.CompletedAt.Sub(job.StartTime)
				if pipelineCtx.Metrics.AverageSynthesisTime == 0 {
					pipelineCtx.Metrics.AverageSynthesisTime = synthesisTime
				} else {
					// Moving average
					pipelineCtx.Metrics.AverageSynthesisTime =
						(pipelineCtx.Metrics.AverageSynthesisTime + synthesisTime) / 2
				}
			}

			// Send completed job to sequencer
			select {
			case pipelineCtx.completedJobs <- job:
			case <-ctx.Done():
				return
			}
		}
	}
}

// audioSequencer ensures audio chunks are delivered in correct order
func (sap *StreamingAudioPipeline) audioSequencer(ctx context.Context, pipelineCtx *PipelineContext) {
	pipelineCtx.wg.Add(1)
	defer pipelineCtx.wg.Done()

	pendingJobs := make(map[int]*SynthesisJob)
	nextSequenceID := 0

	for {
		select {
		case <-ctx.Done():
			return

		case job, ok := <-pipelineCtx.completedJobs:
			if !ok {
				return
			}

			// Handle end-of-stream marker
			if job.Phrase == "" {
				// Send final audio chunk
				finalChunk := &AudioChunk{
					SequenceID: job.ID,
					IsLast:     true,
					Timestamp:  time.Now(),
					Phrase:     "", // Empty phrase for final marker
				}

				select {
				case pipelineCtx.AudioChunks <- finalChunk:
				case <-ctx.Done():
				}
				return
			}

			// Store job in pending map
			pendingJobs[job.ID] = job

			// Process all consecutive jobs starting from nextSequenceID
			for {
				pendingJob, exists := pendingJobs[nextSequenceID]
				if !exists {
					break
				}

				// Create audio chunk
				if pendingJob.Error == nil && pendingJob.Result != nil {
					chunk := &AudioChunk{
						Audio:       pendingJob.Result.Audio,
						ContentType: pendingJob.Result.ContentType,
						Length:      pendingJob.Result.Length,
						SequenceID:  pendingJob.ID,
						IsLast:      false,
						Phrase:      pendingJob.Phrase,
						Timestamp:   time.Now(),
					}

					// Record first audio timing
					if pipelineCtx.Metrics.FirstAudioTime.IsZero() {
						pipelineCtx.Metrics.FirstAudioTime = time.Now()
					}

					// Send audio chunk
					select {
					case pipelineCtx.AudioChunks <- chunk:
					case <-ctx.Done():
						return
					}

					log.Printf("🎵 Audio chunk %d delivered: %s", pendingJob.ID, pendingJob.Phrase)
				}

				// Remove processed job
				delete(pendingJobs, nextSequenceID)
				nextSequenceID++
			}
		}
	}
}

// GetPipelineMetrics returns performance metrics for a pipeline context
func (sap *StreamingAudioPipeline) GetPipelineMetrics(contextID string) (*PipelineMetrics, error) {
	sap.mu.RLock()
	defer sap.mu.RUnlock()

	pipelineCtx, exists := sap.activeContexts[contextID]
	if !exists {
		return nil, fmt.Errorf("pipeline context %s not found", contextID)
	}

	pipelineCtx.mu.RLock()
	defer pipelineCtx.mu.RUnlock()

	// Create a copy of metrics
	metrics := *pipelineCtx.Metrics
	return &metrics, nil
}

// GetActivePipelines returns a list of active pipeline context IDs
func (sap *StreamingAudioPipeline) GetActivePipelines() []string {
	sap.mu.RLock()
	defer sap.mu.RUnlock()

	contexts := make([]string, 0, len(sap.activeContexts))
	for contextID := range sap.activeContexts {
		contexts = append(contexts, contextID)
	}

	return contexts
}

// UpdateTTSOptions updates the TTS options for new pipelines
func (sap *StreamingAudioPipeline) UpdateTTSOptions(options *TTSOptions) {
	sap.mu.Lock()
	defer sap.mu.Unlock()
	sap.ttsOptions = options
}