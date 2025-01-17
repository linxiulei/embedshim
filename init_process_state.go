/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package embedshim

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/runtime"
	google_protobuf "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// CheckpointConfig holds task checkpoint configuration
type CheckpointConfig struct {
	WorkDir                  string
	Path                     string
	Exit                     bool
	AllowOpenTCP             bool
	AllowExternalUnixSockets bool
	AllowTerminal            bool
	FileLocks                bool
	EmptyNamespaces          []string
}

type initState interface {
	Start(context.Context) error
	Delete(context.Context) error
	Pause(context.Context) error
	Resume(context.Context) error
	Update(context.Context, *google_protobuf.Any) error
	Checkpoint(context.Context, *CheckpointConfig) error
	Exec(context.Context, string, runtime.ExecOpts) (runtime.Process, error)
	Kill(context.Context, uint32, bool) error
	SetExited(int)
	Status(context.Context) (string, error)
}

type createdState struct {
	p *initProcess
}

func (s *createdState) transition(name string) error {
	switch name {
	case "running":
		s.p.initState = &runningState{p: s.p}
	case "stopped":
		s.p.initState = &stoppedState{p: s.p}
	case "deleted":
		s.p.initState = &deletedState{}
	default:
		return fmt.Errorf("invalid state transition %q to %q", stateName(s), name)
	}
	return nil
}

func (s *createdState) Pause(_ context.Context) error {
	return fmt.Errorf("cannot pause task in created state")
}

func (s *createdState) Resume(_ context.Context) error {
	return fmt.Errorf("cannot resume task in created state")
}

func (s *createdState) Update(ctx context.Context, r *google_protobuf.Any) error {
	return s.p.update(ctx, r)
}

func (s *createdState) Checkpoint(_ context.Context, _ *CheckpointConfig) error {
	return fmt.Errorf("cannot checkpoint a task in created state")
}

func (s *createdState) Start(ctx context.Context) error {
	if err := s.p.start(ctx); err != nil {
		return err
	}
	return s.transition("running")
}

func (s *createdState) Delete(ctx context.Context) error {
	if err := s.p.delete(ctx); err != nil {
		return err
	}
	return s.transition("deleted")
}

func (s *createdState) Kill(ctx context.Context, sig uint32, all bool) error {
	return s.p.kill(ctx, sig, all)
}

func (s *createdState) SetExited(status int) {
	s.p.setExited(status)

	if err := s.transition("stopped"); err != nil {
		panic(err)
	}
}

func (s *createdState) Exec(ctx context.Context, id string, opts runtime.ExecOpts) (runtime.Process, error) {
	return s.p.exec(ctx, id, opts)
}

func (s *createdState) Status(_ context.Context) (string, error) {
	return "created", nil
}

type runningState struct {
	p *initProcess
}

func (s *runningState) transition(name string) error {
	switch name {
	case "stopped":
		s.p.initState = &stoppedState{p: s.p}
	case "paused":
		s.p.initState = &pausedState{p: s.p}
	default:
		return fmt.Errorf("invalid state transition %q to %q", stateName(s), name)
	}
	return nil
}

func (s *runningState) Pause(_ context.Context) error {
	return fmt.Errorf("pause not implemented yet")
}

func (s *runningState) Resume(_ context.Context) error {
	return fmt.Errorf("cannot resume a running process")
}

func (s *runningState) Update(ctx context.Context, r *google_protobuf.Any) error {
	return s.p.update(ctx, r)
}

func (s *runningState) Checkpoint(_ context.Context, _ *CheckpointConfig) error {
	return fmt.Errorf("checkpoint not implemented yet")
}

func (s *runningState) Start(_ context.Context) error {
	return fmt.Errorf("cannot start a running process")
}

func (s *runningState) Delete(_ context.Context) error {
	return fmt.Errorf("cannot delete a running process")
}

func (s *runningState) Kill(ctx context.Context, sig uint32, all bool) error {
	return s.p.kill(ctx, sig, all)
}

func (s *runningState) SetExited(status int) {
	s.p.setExited(status)

	if err := s.transition("stopped"); err != nil {
		panic(err)
	}
}

func (s *runningState) Exec(ctx context.Context, id string, opts runtime.ExecOpts) (runtime.Process, error) {
	return s.p.exec(ctx, id, opts)
}

func (s *runningState) Status(_ context.Context) (string, error) {
	return "running", nil
}

type pausedState struct {
	p *initProcess
}

func (s *pausedState) transition(name string) error {
	switch name {
	case "running":
		s.p.initState = &runningState{p: s.p}
	case "stopped":
		s.p.initState = &stoppedState{p: s.p}
	default:
		return fmt.Errorf("invalid state transition %q to %q", stateName(s), name)
	}
	return nil
}

func (s *pausedState) Pause(_ context.Context) error {
	return fmt.Errorf("cannot pause a paused container")
}

func (s *pausedState) Resume(_ context.Context) error {
	return fmt.Errorf("resume not implemented yet")
}

func (s *pausedState) Update(ctx context.Context, r *google_protobuf.Any) error {
	return s.p.update(ctx, r)
}

func (s *pausedState) Checkpoint(_ context.Context, _ *CheckpointConfig) error {
	return fmt.Errorf("checkpoint not implemented yet")
}

func (s *pausedState) Start(_ context.Context) error {
	return fmt.Errorf("cannot start a paused process")
}

func (s *pausedState) Delete(_ context.Context) error {
	return fmt.Errorf("cannot delete a paused process")
}

func (s *pausedState) Kill(ctx context.Context, sig uint32, all bool) error {
	return s.p.kill(ctx, sig, all)
}

func (s *pausedState) SetExited(status int) {
	s.p.setExited(status)

	if err := s.p.runtime.Resume(context.Background(), s.p.ID()); err != nil {
		logrus.WithError(err).Error("resuming exited container from paused state")
	}

	if err := s.transition("stopped"); err != nil {
		panic(err)
	}
}

func (s *pausedState) Exec(_ context.Context, _ string, _ runtime.ExecOpts) (runtime.Process, error) {
	return nil, fmt.Errorf("cannot exec in a paused state")
}

func (s *pausedState) Status(_ context.Context) (string, error) {
	return "paused", nil
}

type stoppedState struct {
	p *initProcess
}

func (s *stoppedState) transition(name string) error {
	switch name {
	case "deleted":
		s.p.initState = &deletedState{}
	default:
		return fmt.Errorf("invalid state transition %q to %q", stateName(s), name)
	}
	return nil
}

func (s *stoppedState) Pause(_ context.Context) error {
	return fmt.Errorf("cannot pause a stopped container")
}

func (s *stoppedState) Resume(_ context.Context) error {
	return fmt.Errorf("cannot resume a stopped container")
}

func (s *stoppedState) Update(_ context.Context, _ *google_protobuf.Any) error {
	return fmt.Errorf("cannot update a stopped container")
}

func (s *stoppedState) Checkpoint(_ context.Context, _ *CheckpointConfig) error {
	return fmt.Errorf("cannot checkpoint a stopped container")
}

func (s *stoppedState) Start(_ context.Context) error {
	return fmt.Errorf("cannot start a stopped process")
}

func (s *stoppedState) Delete(ctx context.Context) error {
	if err := s.p.delete(ctx); err != nil {
		return err
	}
	return s.transition("deleted")
}

func (s *stoppedState) Kill(ctx context.Context, sig uint32, all bool) error {
	return s.p.kill(ctx, sig, all)
}

func (s *stoppedState) SetExited(_ int) {
	// no op
}

func (s *stoppedState) Exec(_ context.Context, _ string, _ runtime.ExecOpts) (runtime.Process, error) {
	return nil, fmt.Errorf("cannot exec in a stopped state")
}

func (s *stoppedState) Status(_ context.Context) (string, error) {
	return "stopped", nil
}

func stateName(v interface{}) string {
	// TODO: add exec state
	switch v.(type) {
	case *runningState:
		return "running"
	case *createdState:
		return "created"
	case *deletedState:
		return "deleted"
	case *stoppedState:
		return "stopped"
	}
	panic(errors.Errorf("invalid state %v", v))
}
