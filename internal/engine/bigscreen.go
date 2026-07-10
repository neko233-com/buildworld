package engine

import (
	"database/sql"
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

func safeCount(db *sql.DB, q string, args ...interface{}) int {
	var n int
	_ = db.QueryRow(q, args...).Scan(&n)
	return n
}

func safeQuery(db *sql.DB, q string, args ...interface{}) *sql.Rows {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil
	}
	return rows
}

func (s *BigScreenService) GetBigScreenData() (*BigScreenData, error) {
	data := &BigScreenData{
		CurrentTime:   time.Now().Format("2006-01-02 15:04:05"),
		RecentBuilds:  []BigScreenBuild{},
		AgentStatus:   []BigScreenAgent{},
		ProjectStats:  []BigScreenProjectStat{},
		TrendData:     []BigScreenTrendPoint{},
		Notifications: []BigScreenNotif{},
	}

	db := s.store.DB()
	today := time.Now().Format("2006-01-02")

	totalBuilds := safeCount(db, "SELECT COUNT(*) FROM builds")
	runningBuilds := safeCount(db, "SELECT COUNT(*) FROM builds WHERE status = 'running'")
	queuedBuilds := safeCount(db, "SELECT COUNT(*) FROM build_queue_items WHERE status = 'queued'")
	successToday := safeCount(db, "SELECT COUNT(*) FROM builds WHERE status = 'success' AND date(started_at) = ?", today)
	failedToday := safeCount(db, "SELECT COUNT(*) FROM builds WHERE status = 'failed' AND date(started_at) = ?", today)

	totalFinished := successToday + failedToday
	successRate := 0.0
	if totalFinished > 0 {
		successRate = float64(successToday) / float64(totalFinished) * 100
	}

	totalProjects := safeCount(db, "SELECT COUNT(*) FROM projects")
	totalAgents := safeCount(db, "SELECT COUNT(*) FROM workers")
	activeAgents := safeCount(db, "SELECT COUNT(*) FROM workers WHERE status = 'online'")

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

	if rows := safeQuery(db, `SELECT b.id, b.number, p.name, b.status,
		COALESCE(b.branch, ''), COALESCE(b.duration_ms, 0),
		COALESCE(b.started_at, b.created_at)
		FROM builds b JOIN projects p ON b.project_id = p.id
		ORDER BY b.id DESC LIMIT 20`); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var b BigScreenBuild
			var started time.Time
			if err := rows.Scan(&b.ID, &b.Number, &b.Project, &b.Status, &b.Branch, &b.Duration, &started); err == nil {
				b.StartedAt = started.Format("2006-01-02 15:04:05")
				data.RecentBuilds = append(data.RecentBuilds, b)
			}
		}
	}

	_ = s.store.MarkOfflineWorkers()
	if workers, err := s.store.ListWorkers(); err == nil {
		for _, w := range workers {
			data.AgentStatus = append(data.AgentStatus, BigScreenAgent{
				Name:      w.Name,
				Status:    w.Status,
				Pool:      w.Pool,
				MaxBuilds: w.MaxConcurrentBuilds,
			})
		}
	}

	if rows := safeQuery(db, `SELECT p.name,
		COUNT(b.id) as total,
		COALESCE(SUM(CASE WHEN b.status = 'success' THEN 1 ELSE 0 END), 0) as success_cnt,
		COALESCE((SELECT status FROM builds WHERE project_id = p.id ORDER BY id DESC LIMIT 1), '')
		FROM projects p
		LEFT JOIN builds b ON b.project_id = p.id
		GROUP BY p.id
		ORDER BY p.id DESC LIMIT 10`); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var ps BigScreenProjectStat
			var total, success int
			if err := rows.Scan(&ps.Name, &total, &success, &ps.LastStatus); err == nil {
				ps.TotalBuilds = total
				if total > 0 {
					ps.SuccessRate = float64(success) / float64(total) * 100
				}
				data.ProjectStats = append(data.ProjectStats, ps)
			}
		}
	}

	if rows := safeQuery(db, `SELECT date, COALESCE(SUM(success_count), 0), COALESCE(SUM(failed_count), 0), 0
		FROM build_stats
		WHERE date >= date('now', '-7 days')
		GROUP BY date
		ORDER BY date ASC`); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var tp BigScreenTrendPoint
			if err := rows.Scan(&tp.Date, &tp.Success, &tp.Failed, &tp.Running); err == nil {
				data.TrendData = append(data.TrendData, tp)
			}
		}
	}

	uptime := time.Since(s.startTime)
	data.SystemMetrics = BigScreenSystem{
		Goroutines: runtime.NumGoroutine(),
		Uptime:     uptime.Round(time.Second).String(),
		GoVersion:  runtime.Version(),
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		CPUs:       runtime.NumCPU(),
	}

	if rows := safeQuery(db, `SELECT COALESCE(nc.name, 'unknown'), COALESCE(ne.event_type, ''), COALESCE(ne.status, ''), ne.created_at
		FROM notification_events ne
		LEFT JOIN notification_channels nc ON ne.channel_id = nc.id
		ORDER BY ne.id DESC LIMIT 10`); rows != nil {
		defer rows.Close()
		for rows.Next() {
			var n BigScreenNotif
			var t time.Time
			if err := rows.Scan(&n.Channel, &n.Event, &n.Status, &t); err == nil {
				n.Time = t.Format("2006-01-02 15:04:05")
				data.Notifications = append(data.Notifications, n)
			}
		}
	}

	return data, nil
}
