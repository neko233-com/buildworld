package worker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/neko233-com/buildworld/internal/workercontrol"
)

type daemonState struct {
	Server     string                    `json:"server"`
	ID         string                    `json:"id"`
	Token      string                    `json:"token"`
	Capacity   int                       `json:"capacity"`
	Draining   bool                      `json:"draining"`
	Completion *workercontrol.Completion `json:"completion,omitempty"`
	Update     *updatePlan               `json:"update,omitempty"`
}

func atomicJSON(name string, value any) error {
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(name), ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), name)
}

func (d *Daemon) loadState() error {
	raw, err := os.ReadFile(filepath.Join(d.stateRoot, "state.json"))
	if errors.Is(err, os.ErrNotExist) {
		d.state = daemonState{Server: d.server, Capacity: d.maxBuilds}
		return nil
	}
	if err != nil {
		return err
	}
	if len(raw) > 512<<10 || json.Unmarshal(raw, &d.state) != nil || d.state.Server != d.server || d.state.ID == "" || d.state.Token == "" {
		return errors.New("worker state is invalid or belongs to another controller; preserve it for inspection")
	}
	if err = d.executor.Configure(d.state.Capacity, d.state.Draining); err != nil {
		return err
	}
	d.id, d.token = d.state.ID, d.state.Token
	return nil
}

func (d *Daemon) saveState() error {
	snapshot := d.executor.Snapshot()
	d.state.Server, d.state.ID, d.state.Token = d.server, d.id, d.token
	d.state.Capacity, d.state.Draining = snapshot.Capacity, snapshot.Draining
	return atomicJSON(filepath.Join(d.stateRoot, "state.json"), &d.state)
}

func (d *Daemon) control(op *workercontrol.Operation) error {
	if op == nil {
		return nil
	}
	if d.state.Completion != nil && d.state.Completion.ID == op.ID {
		return nil
	}
	if op.Kind == "update" {
		return d.stageUpdate(op)
	}
	d.state.Completion = d.executor.executeControl(op)
	return d.saveState()
}
