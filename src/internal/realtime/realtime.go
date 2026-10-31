package realtime

import (
	panelv1 "dockium/gen/panel/v1"
	"dockium/internal/db"
	"dockium/internal/events"
	"dockium/internal/present"
)

type InstanceEvent struct {
	Snapshot  []*db.Instance
	Instance  *db.Instance
	RemovedID int64
}

type AgentInfo struct {
	Agent          *db.Agent
	InstancesTotal uint32
	Running        uint32
}

type AgentEvent struct {
	Snapshot  []*AgentInfo
	Agent     *AgentInfo
	RemovedID int64
}

type Publisher struct {
	instances *events.Broker[InstanceEvent]
	agents    *events.Broker[AgentEvent]
	security  *events.Broker[*panelv1.SecurityEvent]
}

func NewPublisher() *Publisher {
	return &Publisher{
		instances: events.NewBroker[InstanceEvent](),
		agents:    events.NewBroker[AgentEvent](),
		security:  events.NewBroker[*panelv1.SecurityEvent](),
	}
}

func (p *Publisher) InstanceUpdated(instance *db.Instance) {
	if instance == nil {
		return
	}
	snapshot := *instance
	p.instances.Publish(InstanceEvent{Instance: &snapshot})
}

func (p *Publisher) InstanceRemoved(id int64) {
	p.instances.Publish(InstanceEvent{RemovedID: id})
}

func (p *Publisher) InstancesSnapshot(items []*db.Instance) {
	p.instances.Publish(InstanceEvent{Snapshot: items})
}

func (p *Publisher) AgentUpdated(agent *db.Agent, total, running uint32) {
	if agent == nil {
		return
	}
	snapshot := *agent
	p.agents.Publish(AgentEvent{Agent: &AgentInfo{Agent: &snapshot, InstancesTotal: total, Running: running}})
}

func (p *Publisher) AgentsSnapshot(items []*AgentInfo) {
	p.agents.Publish(AgentEvent{Snapshot: items})
}

func (p *Publisher) AgentRemoved(id int64) {
	p.agents.Publish(AgentEvent{RemovedID: id})
}

func (p *Publisher) SubscribeInstances(buffer int) (<-chan InstanceEvent, func()) {
	return p.instances.Subscribe(buffer)
}

func (p *Publisher) SubscribeAgents(buffer int) (<-chan AgentEvent, func()) {
	return p.agents.Subscribe(buffer)
}

func (p *Publisher) SubscribeSecurity(buffer int) (<-chan *panelv1.SecurityEvent, func()) {
	return p.security.Subscribe(buffer)
}

func (p *Publisher) LoginLogged(entry *db.LoginLog) {
	p.security.Publish(&panelv1.SecurityEvent{Payload: &panelv1.SecurityEvent_LoginLog{LoginLog: present.LoginLog(entry)}})
}

func (p *Publisher) ApiRequested(entry *db.ApiRequest) {
	p.security.Publish(&panelv1.SecurityEvent{Payload: &panelv1.SecurityEvent_ApiRequest{ApiRequest: present.ApiRequest(entry)}})
}

func (p *Publisher) BanChanged(ban *db.Ban) {
	p.security.Publish(&panelv1.SecurityEvent{Payload: &panelv1.SecurityEvent_Ban{Ban: present.Ban(ban)}})
}

func (p *Publisher) BanRemoved(id int64) {
	p.security.Publish(&panelv1.SecurityEvent{Payload: &panelv1.SecurityEvent_BanRemoved{BanRemoved: &panelv1.BanRemoved{Id: id}}})
}

func (p *Publisher) SessionRevoked(session *db.SessionRow) {
	p.security.Publish(&panelv1.SecurityEvent{Payload: &panelv1.SecurityEvent_SessionRevoked{SessionRevoked: present.SessionInfo(session, false)}})
}
