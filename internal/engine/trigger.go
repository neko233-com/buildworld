package engine

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/buildworld233/internal/store"
	"github.com/neko233-com/buildworld233/internal/ws"
)

type TriggerChecker struct {
	store  *store.Store
	runner *BuildRunner
	hub    *ws.Hub
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewTriggerChecker(s *store.Store, r *BuildRunner, h *ws.Hub) *TriggerChecker {
	return &TriggerChecker{
		store:  s,
		runner: r,
		hub:    h,
		stopCh: make(chan struct{}),
	}
}

func (tc *TriggerChecker) Start() {
	tc.wg.Add(2)
	go tc.cronChecker()
	go tc.vcsPoller()
}

func (tc *TriggerChecker) Stop() {
	close(tc.stopCh)
	tc.wg.Wait()
}

func (tc *TriggerChecker) cronChecker() {
	defer tc.wg.Done()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-tc.stopCh:
			return
		case <-ticker.C:
			tc.checkCronTriggers()
		}
	}
}

func (tc *TriggerChecker) checkCronTriggers() {
	projects, err := tc.store.ListProjects()
	if err != nil {
		return
	}
	now := time.Now()
	for _, p := range projects {
		cfg, err := tc.loadProjectConfig(p)
		if err != nil {
			continue
		}
		for _, t := range cfg.Triggers {
			if t.Type != "schedule" {
				continue
			}
			cronExpr := t.Config["cron"]
			if cronExpr == "" {
				continue
			}
			if matchCron(cronExpr, now) {
				tc.triggerProjectBuild(p, "schedule", p.DefaultBranch, "")
			}
		}
	}
}

func (tc *TriggerChecker) vcsPoller() {
	defer tc.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-tc.stopCh:
			return
		case <-ticker.C:
			tc.pollVCSRoots()
		}
	}
}

func (tc *TriggerChecker) pollVCSRoots() {
	roots, err := tc.store.ListVCSRoots()
	if err != nil {
		return
	}
	for _, root := range roots {
		if root.PollInterval <= 0 {
			continue
		}
		sha, err := gitLSRemote(root.URL, root.Branch)
		if err != nil {
			continue
		}
		var cfg map[string]interface{}
		json.Unmarshal([]byte(root.Config), &cfg)
		if cfg == nil {
			cfg = map[string]interface{}{}
		}
		lastSHA, _ := cfg["last_commit_sha"].(string)
		if lastSHA == sha {
			continue
		}
		cfg["last_commit_sha"] = sha
		newCfg, _ := json.Marshal(cfg)
		tc.store.UpdateVCSRootConfig(root.ID, string(newCfg))
		if lastSHA != "" {
			projects, err := tc.store.ListProjectsByVCSRoot(root.ID)
			if err != nil {
				continue
			}
			for _, p := range projects {
				cfg, err := tc.loadProjectConfig(p)
				if err != nil {
					continue
				}
				hasVCSTrigger := false
				for _, t := range cfg.Triggers {
					if t.Type == "vcs" {
						hasVCSTrigger = true
						break
					}
				}
				if hasVCSTrigger {
					tc.triggerProjectBuild(p, "vcs", root.Branch, sha)
				}
			}
		}
	}
}

func (tc *TriggerChecker) HandleBuildFinish(build *store.Build) {
	if build.Status != "success" {
		return
	}
	projects, err := tc.store.ListProjects()
	if err != nil {
		return
	}
	for _, p := range projects {
		cfg, err := tc.loadProjectConfig(p)
		if err != nil {
			continue
		}
		for _, t := range cfg.Triggers {
			if t.Type != "finish" {
				continue
			}
			depProjectIDStr := t.Config["project_id"]
			if depProjectIDStr == "" {
				continue
			}
			depProjectID, err := strconv.ParseInt(depProjectIDStr, 10, 64)
			if err != nil {
				continue
			}
			if depProjectID != build.ProjectID {
				continue
			}
			branch := t.Config["branch"]
			tc.triggerProjectBuild(p, "finish", branch, "")
		}
	}
}

func (tc *TriggerChecker) loadProjectConfig(p *store.Project) (*BuildConfig, error) {
	cfg, err := ParsePipelineConfig(p.Config)
	if err != nil {
		return nil, err
	}
	if p.TemplateID != nil {
		tmpl, err := tc.store.GetBuildTemplate(*p.TemplateID)
		if err == nil {
			tmplCfg, err := ParseBuildConfig(tmpl.Config)
			if err == nil {
				cfg = MergeBuildConfig(tmplCfg, cfg)
			}
		}
	}
	return cfg, nil
}

func (tc *TriggerChecker) triggerProjectBuild(p *store.Project, trigger, branch, commitSHA string) {
	if branch == "" {
		branch = p.DefaultBranch
	}
	num, err := tc.store.NextBuildNumber(p.ID)
	if err != nil {
		return
	}
	build, err := tc.store.CreateBuild(p.ID, num, trigger, branch, commitSHA, "", nil, nil)
	if err != nil {
		return
	}
	if tc.runner != nil {
		tc.runner.Run(build.ID)
	}
}

func gitLSRemote(url, branch string) (string, error) {
	if branch == "" {
		branch = "HEAD"
	}
	cmd := exec.Command("git", "ls-remote", url, "refs/heads/"+branch)
	out, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("git", "ls-remote", url, branch)
		out, err = cmd.Output()
		if err != nil {
			return "", fmt.Errorf("git ls-remote: %w", err)
		}
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return "", fmt.Errorf("no output from git ls-remote")
	}
	return parts[0], nil
}

func matchCron(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false
	}
	minute := t.Minute()
	hour := t.Hour()
	dom := t.Day()
	month := int(t.Month())
	dow := int(t.Weekday())
	return matchCronField(fields[0], minute, 0, 59) &&
		matchCronField(fields[1], hour, 0, 23) &&
		matchCronField(fields[2], dom, 1, 31) &&
		matchCronField(fields[3], month, 1, 12) &&
		matchCronField(fields[4], dow, 0, 6)
}

func matchCronField(field string, val, min, max int) bool {
	if field == "*" {
		return true
	}
	for _, part := range strings.Split(field, ",") {
		if strings.HasPrefix(part, "*/") {
			n, err := strconv.Atoi(part[2:])
			if err == nil && n > 0 && val%n == 0 {
				return true
			}
			continue
		}
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(rangeParts[0])
			hi, err2 := strconv.Atoi(rangeParts[1])
			if err1 == nil && err2 == nil && val >= lo && val <= hi {
				return true
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err == nil && n == val {
			return true
		}
	}
	return false
}
