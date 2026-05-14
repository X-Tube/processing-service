package video

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/X-Tube/processing-service/internal/observability"
)

type Segmenter interface {
	Segment(ctx context.Context, inputPath, outputDir string, profiles []Profile) ([]Segment, error)
}

type FFmpegTranscoder struct {
	config TranscoderConfig
	logger *slog.Logger
}

type TranscoderConfig struct {
	SegmentSeconds int
	ChunkDetails   bool
	FFmpegProgress bool
	LogLevel       string
}

func NewFFmpegTranscoder(config TranscoderConfig, logger *slog.Logger) *FFmpegTranscoder {
	if logger == nil {
		logger = slog.Default()
	}

	return &FFmpegTranscoder{
		config: config,
		logger: logger,
	}
}

func (t *FFmpegTranscoder) Segment(ctx context.Context, inputPath, outputDir string, profiles []Profile) ([]Segment, error) {
	segmentSeconds := t.config.SegmentSeconds
	if segmentSeconds <= 0 {
		segmentSeconds = 10
	}

	var segments []Segment

	for _, profile := range profiles {
		profileStartedAt := time.Now()
		profileDir := filepath.Join(outputDir, profile.Name)
		if err := os.MkdirAll(profileDir, 0o755); err != nil {
			return nil, fmt.Errorf("create output directory for %s: %w", profile.Name, err)
		}

		pattern := filepath.Join(profileDir, "video-%d.mp4")
		filter := fmt.Sprintf("scale=-2:trunc(min(%d\\,ih)/2)*2", profile.Height)
		args := []string{
			"-hide_banner",
			"-y",
			"-nostats",
			"-stats_period", "10",
			"-progress", "pipe:1",
			"-i", inputPath,
			"-vf", filter,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-crf", "23",
			"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", segmentSeconds),
			"-c:a", "aac",
			"-movflags", "+faststart",
			"-f", "segment",
			"-segment_time", fmt.Sprintf("%d", segmentSeconds),
			"-segment_start_number", "1",
			"-reset_timestamps", "1",
			pattern,
		}

		var stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stderr = &stderr

		t.logger.Info("video resolution transcode started", "component", "ffmpeg", "resolution", profile.Name, "height", profile.Height, "segment_seconds", segmentSeconds)
		if err := t.runFFmpeg(ctx, cmd, profile, profileStartedAt); err != nil {
			trimmedOutput := trimCommandOutput(stderr.String())
			t.logger.Error("video resolution transcode failed", "component", "ffmpeg", "resolution", profile.Name, "height", profile.Height, "duration_ms", observability.DurationMillis(profileStartedAt), "stderr", trimmedOutput, "error", err)
			return nil, fmt.Errorf("generate %s segments with ffmpeg: %w: %s", profile.Name, err, trimmedOutput)
		}

		files, err := filepath.Glob(filepath.Join(profileDir, "video-*.mp4"))
		if err != nil {
			return nil, fmt.Errorf("list generated %s segments: %w", profile.Name, err)
		}
		if len(files) == 0 {
			return nil, fmt.Errorf("ffmpeg generated no segments for %s", profile.Name)
		}

		sort.Slice(files, func(i, j int) bool {
			return segmentNumber(files[i]) < segmentNumber(files[j])
		})

		for _, file := range files {
			index := segmentNumber(file)
			sizeBytes, err := fileSize(file)
			if err != nil {
				return nil, fmt.Errorf("stat generated %s segment: %w", profile.Name, err)
			}
			if t.config.ChunkDetails {
				t.logger.Debug("video segment file discovered", "component", "ffmpeg", "resolution", profile.Name, "chunk_index", index, "file_name", filepath.Base(file), "size_bytes", sizeBytes)
			}
			segments = append(segments, Segment{
				Profile:  profile.Name,
				Index:    index,
				FilePath: file,
				FileName: filepath.Base(file),
				Size:     sizeBytes,
			})
		}

		t.logger.Info("video resolution transcode finished", "component", "ffmpeg", "resolution", profile.Name, "height", profile.Height, "chunk_count", len(files), "size_bytes", totalSize(segments, profile.Name), "duration_ms", observability.DurationMillis(profileStartedAt))
	}

	return segments, nil
}

func (t *FFmpegTranscoder) runFFmpeg(ctx context.Context, cmd *exec.Cmd, profile Profile, startedAt time.Time) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open ffmpeg progress pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	progressDone := make(chan struct{})
	go func() {
		defer close(progressDone)
		t.logProgress(ctx, stdout, profile, startedAt)
	}()

	err = cmd.Wait()
	<-progressDone

	return err
}

func (t *FFmpegTranscoder) logProgress(ctx context.Context, output io.Reader, profile Profile, startedAt time.Time) {
	if !t.config.FFmpegProgress {
		_, _ = io.Copy(io.Discard, output)
		return
	}

	scanner := bufio.NewScanner(output)
	values := map[string]string{}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		values[key] = value
		if key != "progress" {
			continue
		}

		t.logger.Log(
			ctx,
			t.progressLogLevel(),
			"ffmpeg profile progress",
			"component", "ffmpeg",
			"resolution", profile.Name,
			"height", profile.Height,
			"ffmpeg_status", value,
			"frame", values["frame"],
			"fps", values["fps"],
			"out_time", values["out_time"],
			"speed", values["speed"],
			"duration_ms", observability.DurationMillis(startedAt),
		)

		values = map[string]string{}
	}

	if err := scanner.Err(); err != nil {
		t.logger.Warn("ffmpeg progress read failed", "component", "ffmpeg", "resolution", profile.Name, "height", profile.Height, "error", err)
	}
}

func (t *FFmpegTranscoder) progressLogLevel() slog.Level {
	if strings.EqualFold(strings.TrimSpace(t.config.LogLevel), "debug") {
		return slog.LevelDebug
	}

	return slog.LevelInfo
}

func fileSize(filePath string) (int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, err
	}

	return info.Size(), nil
}

func totalSize(segments []Segment, profile string) int64 {
	var total int64
	for _, segment := range segments {
		if segment.Profile == profile {
			total += segment.Size
		}
	}

	return total
}

func segmentNumber(filePath string) int {
	fileName := filepath.Base(filePath)
	fileName = strings.TrimPrefix(fileName, "video-")
	fileName = strings.TrimSuffix(fileName, ".mp4")

	number, err := strconv.Atoi(fileName)
	if err != nil {
		return 0
	}

	return number
}

func trimCommandOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 2000 {
		return output
	}

	return output[len(output)-2000:]
}
