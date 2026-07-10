package engine

import (
	"runtime"
	"time"

	"github.com/neko233-com/buildworld233/internal/store"
)

type BigScreenService struct {
	store     *store.Store
	startTime time.Time
}

func NewBigScreenService(s *store.Store) *BigScreenService {
	return &BigScreenService{store: s, startTime: time.Now()}
}

type BigScreenData struct {
	Summary       BigScreenSummary       `json:"summary"`
	RecentBuilds  []BigScreenBuild       `json:"recent_builds"`
	AgentStatus   []BigScreenAgent       `json:"agent_status"`
	ProjectStats  []BigScreenProjectStat `json:"project_stats"`
	TrendData     []BigScreenTrendPoint  `json:"trend_data"`
	SystemMetrics BigScreenSystem        `json:"system_metrics"`
	Notifications []BigScreenNotif       `json:"notifications"`
	CurrentTime   string                 `json:"current_time"`
}

type BigScreenSummary struct {
	TotalBuilds   int     `json:"total_builds"`
	RunningBuilds int     `json:"running_builds"`
	QueuedBuilds  int     `json:"queued_builds"`
	SuccessToday  int     `json:"success_today"`
	FailedToday   int     `json:"failed_today"`
	SuccessRate   float64 `json:"success_rate"`
	ActiveAgents  int     `json:"active_agents"`
	TotalAgents   int     `json:"total_agents"`
	TotalProjects int     `json:"total_projects"`
}

type BigScreenBuild struct {
	ID        int64  `json:"id"`
	Number    int    `json:"number"`
	Project   string `json:"project"`
	Status    string `json:"status"`
	Branch    string `json:"branch"`
	Duration  int64  `json:"duration_ms"`
	StartedAt string `json:"started_at"`
}

type BigScreenAgent struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Pool      string `json:"pool"`
	Builds    int    `json:"active_builds"`
	MaxBuilds int    `json:"max_builds"`
}

type BigScreenProjectStat struct {
	Name        string  `json:"name"`
	TotalBuilds int     `json:"total_builds"`
	SuccessRate float64 `json:"success_rate"`
	LastStatus  string  `json:"last_status"`
}

type BigScreenTrendPoint struct {
	Date    string `json:"date"`
	Success int    `json:"success"`
	Failed  int    `json:"failed"`
	Running int    `json:"running"`
}

type BigScreenSystem struct {
	Goroutines int    `json:"goroutines"`
	Uptime     string `json:"uptime"`
	GoVersion  string `json:"go_version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	CPUs       int    `json:"cpus"`
}

type BigScreenNotif struct {
	Channel string `json:"channel"`
	Event   string `json:"event"`
	Status  string `json:"status"`
	Time    string `json:"time"`
}

func (s *BigScreenService) GetBigScreenData() (*BigScreenData, error) {
	data := &BigScreenData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	db := s.store.DB()
	today := time.Now().Format("2006-01-02")

	var totalBuilds, runningBuilds, queuedBuilds, successToday, failedToday int
	db.QueryRow("SELECT COUNT(*) FROM builds").Scan(&totalBuilds)
	db.QueryRow("SELECT COUNT(*) FROM builds WHERE status = 'running'").Scan(&runningBuilds)
	db.QueryRow("SELECT COUNT(*) FROM build_queue_items WHERE status = 'queued'").Scan(&queuedBuilds)
	db.QueryRow("SELECT COUNT(*) FROM builds WHERE status = 'success' AND date(started_at) = ?", today).Scan(&successToday)
	db.QueryRow("SELECT COUNT(*) FROM builds WHERE status = 'failed' AND date(started_at) = ?", today).Scan(&failedToday)

	totalFinished := successToday + failedToday
	successRate := 0.0
	if totalFinished > 0 {
		successRate = float64(successToday) / float64(totalFinished) * 100
	}

	var totalProjects, activeAgents, totalAgents int
	db.QueryRow("SELECT COUNT(*) FROM projects").Scan(&totalProjects)
	db.QueryRow("SELECT COUNT(*) FROM workers").Scan(&totalAgents)
	db.QueryRow("SELECT COUNT(*) FROM workers WHERE status = 'online'").Scan(&activeAgents)

	data.Summary = BigScreenSummary{
		TotalBuilds:   totalBuilds,
		RunningBuilds: runningBuilds,
		QueuedBuilds:  queuedBuilds,
		SuccessToday:  successToday,
		FailedToday:   failedToday,
		SuccessRate:   successRate,
		ActiveAgents:  activeAgents,
		TotalAgents:   totalAgents,
		TotalProjects: totalProjects,
	}

	rows, _ := db.Query(`
		SELECT b.id, b.number, p.name, b.status, COALESCE(b.branch, ''), COALESCE(b.duration_ms, 0), COALESCE(b.started_at, b.created_at)
		FROM builds b JOIN projects p ON b.project_id = p.id
		ORDER BY b.id DESC LIMIT 20
	`)
	data.RecentBuilds = []BigScreenBuild{}
	for rows.Next() {
		var b BigScreenBuild
		var started time.Time
		rows.Scan(&b.ID, &b.Number, &b.Project, &b.Status, &b.Branch, &b.Duration, &started)
		b.StartedAt = started.Format("2006-01-02 15:04:05")
		data.RecentBuilds = append(data.RecentBuilds, b)
	}
	rows.Close()

	_ = s.store.MarkOfflineWorkers()
	workers, _ := s.store.ListWorkers()
	data.AgentStatus = []BigScreenAgent{}
	for _, w := range workers {
		data.AgentStatus = append(data.AgentStatus, BigScreenAgent{
			Name:      w.Name,
			Status:    w.Status,
			Pool:      w.Pool,
			Builds:    0,
			MaxBuilds: w.MaxConcurrentBuilds,
		})
	}

	rows, _ = db.Query(`
		SELECT p.name,
			COUNT(b.id) as total,
			COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success,
			(SELECT status FROM builds WHERE project_id = p.id ORDER BY id DESC LIMIT 1) as last_status
		FROM projects p
		LEFT JOIN builds b ON b.project_id = p.id
		GROUP BY p.id
		ORDER BY p.id DESC LIMIT 10
	`)
	data.ProjectStats = []BigScreenProjectStat{}
	for rows.Next() {
		var ps BigScreenProjectStat
		var total, success int
		var lastStatus string
		rows.Scan(&ps.Name, &total, &success, &lastStatus)
		ps.TotalBuilds = total
		if total > 0 {
			ps.SuccessRate = float64(success) / float64(total) * 100
		}
		ps.LastStatus = lastStatus
		data.ProjectStats = append(data.ProjectStats, ps)
	}
	rows.Close()

	rows, _ = db.Query(`
		SELECT date, COALESCE(SUM(success_count), 0), COALESCE(SUM(failed_count), 0), 0
		FROM build_stats
		WHERE date >= date('now', '-7 days')
		GROUP BY date
		ORDER BY date ASC
	`)
	data.TrendData = []BigScreenTrendPoint{}
	for rows.Next() {
		var tp BigScreenTrendPoint
		rows.Scan(&tp.Date, &tp.Success, &tp.Failed, &tp.Running)
		data.TrendData = append(data.TrendData, tp)
	}
	rows.Close()

	uptime := time.Since(s.startTime)
	uptimeStr := uptime.Round(time.Second).String()
	data.SystemMetrics = BigScreenSystem{
		Goroutines: runtime.NumGoroutine(),
		Uptime:     uptimeStr,
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPUs:       runtime.NumCPU(),
	}

	rows, _ = db.Query(`
		SELECT nc.name, ne.event_type, ne.status, ne.created_at
		FROM notification_events ne
		JOIN notification_channels nc ON ne.channel_id = nc.id
		ORDER BY ne.id DESC LIMIT 10
	`)
	data.Notifications = []BigScreenNotif{}
	for rows.Next() {
		var n BigScreenNotif
		var t time.Time
		rows.Scan(&n.Channel, &n.Event, &n.Status, &t)
		n.Time = t.Format("2006-01-02 15:04:05")
		data.Notifications = append(data.Notifications, n)
	}
	rows.Close()

	return data, nil
}
