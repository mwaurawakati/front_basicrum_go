package backup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/basicrum/front_basicrum_go/types"
	"github.com/eapache/go-resiliency/batcher"
	"github.com/robfig/cron/v3"
)

// SingleFileBackup saves the events on the file system
type SingleFileBackup struct {
	batcher            *batcher.Batcher
	cron               *cron.Cron
	directory          string
	compressionFactory CompressionWriterFactory
}

// NewSingleFileBackup creates single file system backup service
// nolint: revive
func NewSingleFileBackup(
	backupInterval time.Duration,
	directory string,
	compressionFactory CompressionWriterFactory,
) *SingleFileBackup {
	b := batcher.New(backupInterval, func(params []any) error {
		do(params, directory)
		return nil
	})
	c := cron.New()
	result := &SingleFileBackup{
		batcher:            b,
		cron:               c,
		directory:          directory,
		compressionFactory: compressionFactory,
	}
	return result
}

// Compress aggregates hourly files into daily summary with meta
func (b *SingleFileBackup) Compress() {
	day := time.Now().UTC().AddDate(0, 0, -1)
	if err := archiveDay(b.directory, day, b.compressionFactory); err != nil {
		slog.Error(fmt.Sprintf("error archive day[%v] err[%v]", day, err))
	}
}

// SaveAsync saves an event with default batcher
// nolint: revive
func (b *SingleFileBackup) SaveAsync(event *types.Event) {
	go func() {
		forArchiving := event.RequestParameters
		// Flatten headers later
		h, hErr := json.Marshal(forArchiving)
		if hErr != nil {
			slog.Error(hErr.Error())
		}
		forArchiving.Add("request_headers", string(h))
		if err := b.batcher.Run(forArchiving); err != nil {
			slog.Error(fmt.Sprintf("Error archiving expired url[%v] err[%v]", forArchiving, err))
		}
	}()
}

// Flush is called before shutdown to force process of the last batch
func (b *SingleFileBackup) Flush() {
	b.batcher.Shutdown(true)
}
