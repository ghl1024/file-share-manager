/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldapsync

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"

	"github.com/robfig/cron/v3"
)

var (
	GlobalScheduler *Scheduler
	cronParser      = cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
)

type Scheduler struct {
	cron      *cron.Cron
	service   *Service
	configDAO *dao.LDAPConfigDAO
	mutex     sync.Mutex
	entryID   cron.EntryID
}

func NewScheduler() *Scheduler {
	return &Scheduler{
		cron:      cron.New(cron.WithParser(cronParser), cron.WithChain(cron.Recover(cron.DefaultLogger))),
		service:   NewService(),
		configDAO: dao.NewLDAPConfigDAO(),
	}
}

func StartGlobal(ctx context.Context) error {
	scheduler := NewScheduler()
	if err := scheduler.Start(ctx); err != nil {
		return err
	}
	GlobalScheduler = scheduler
	return nil
}

func (s *Scheduler) Start(ctx context.Context) error {
	cfg, err := s.configDAO.Get()
	if err != nil {
		return err
	}
	if err := s.Update(cfg); err != nil {
		return err
	}
	s.cron.Start()
	go func() {
		<-ctx.Done()
		stopCtx := s.cron.Stop()
		select {
		case <-stopCtx.Done():
		case <-time.After(10 * time.Second):
			logger.Warn("ldap_sync_scheduler_stop_timeout")
		}
	}()
	return nil
}

func (s *Scheduler) Update(cfg *model.LDAPConfig) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.entryID != 0 {
		s.cron.Remove(s.entryID)
		s.entryID = 0
	}
	if cfg == nil || cfg.Status != 1 {
		return nil
	}
	spec := strings.TrimSpace(cfg.SyncCron)
	if spec == "" {
		spec = DefaultCron
	}
	if err := ValidateSpec(spec); err != nil {
		return err
	}
	entryID, err := s.cron.AddFunc(spec, func() {
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		if _, err := s.service.Run(ctx, syncTypeAuto); err != nil && !errors.Is(err, ErrNotEnabled) && !errors.Is(err, ErrAlreadyRunning) {
			logger.Warn("ldap_sync_scheduled_run_failed", "error", err)
		}
	})
	if err != nil {
		return err
	}
	s.entryID = entryID
	return nil
}

func UpdateGlobal(cfg *model.LDAPConfig) error {
	if GlobalScheduler == nil {
		return nil
	}
	return GlobalScheduler.Update(cfg)
}

func ValidateSpec(spec string) error {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	_, err := cronParser.Parse(spec)
	return err
}
