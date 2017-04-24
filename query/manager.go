/*
   Copyright (c) 2016, Percona LLC and/or its affiliates. All rights reserved.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package query

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
	mongoExecutor "github.com/percona/qan-agent/query/executor/mongo"
	mysqlExecutor "github.com/percona/qan-agent/query/executor/mysql"
)

const (
	SERVICE_NAME = "query"
)

type Manager struct {
	logger       *pct.Logger
	instanceRepo *instance.Repo
	connFactory  mysql.ConnectionFactory
	// --
	executors map[string]Executor
	running   bool
	sync.Mutex
	status *pct.Status
}

func NewManager(logger *pct.Logger, instanceRepo *instance.Repo, connFactory mysql.ConnectionFactory) *Manager {
	m := &Manager{
		logger:       logger,
		instanceRepo: instanceRepo,
		connFactory:  connFactory,
		// --
		status: pct.NewStatus([]string{SERVICE_NAME}),
	}
	return m
}

/////////////////////////////////////////////////////////////////////////////
// Interface
/////////////////////////////////////////////////////////////////////////////

func (m *Manager) Start() error {
	m.Lock()
	defer m.Unlock()
	if m.running {
		return pct.ServiceIsRunningError{Service: SERVICE_NAME}
	}

	err := m.loadPlugins()
	if err != nil {
		return err
	}

	m.running = true
	m.logger.Info("Started")
	m.status.Update(SERVICE_NAME, "Idle")
	return nil
}

func (m *Manager) Stop() error {
	// Let user stop this tool in case they don't want agent executing queries.
	m.Lock()
	defer m.Unlock()
	if !m.running {
		return nil
	}

	err := m.unloadPlugins()
	if err != nil {
		return err
	}

	m.running = false
	m.logger.Info("Stopped")
	m.status.Update(SERVICE_NAME, "Stopped")
	return nil
}

func (m *Manager) Handle(cmd *proto.Cmd) *proto.Reply {
	m.Lock()
	defer m.Unlock()

	// Don't query if this tool is stopped.
	if !m.running {
		return cmd.Reply(nil, pct.ServiceIsNotRunningError{})
	}

	m.status.UpdateRe(SERVICE_NAME, "Handling", cmd)
	defer m.status.Update(SERVICE_NAME, "Idle")

	// See which type of subsystem this query is for. Right now we only support
	// MySQL, but this abstraction will make adding other subsystems easy.
	var in proto.Instance
	if err := json.Unmarshal(cmd.Data, &in); err != nil {
		return cmd.Reply(nil, err)
	}

	in, err := m.instanceRepo.Get(in.UUID, false) // false = don't cache
	if err != nil {
		return cmd.Reply(nil, err)
	}

	executor, ok := m.executors[in.Subsystem]
	if !ok {
		return cmd.Reply(nil, fmt.Errorf("can't execute %s queries", in.Subsystem))
	}

	return executor.Handle(cmd, in)
}

func (m *Manager) Status() map[string]string {
	return m.status.All()
}

func (m *Manager) GetConfig() ([]proto.AgentConfig, []error) {
	return nil, nil
}

func (m *Manager) GetDefaults(uuid string) map[string]interface{} {
	return nil
}

// --------------------------------------------------------------------------

func (m *Manager) loadPlugins() error {
	err := m.unloadPlugins()
	if err != nil {
		return err
	}

	m.executors["mysql"] = mysqlExecutor.New()
	m.executors["mongo"] = mongoExecutor.New()
	return nil
}

func (m *Manager) unloadPlugins() error {
	m.executors = map[string]Executor{}
	return nil
}
