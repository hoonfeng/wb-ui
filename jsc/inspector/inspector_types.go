package inspector

// InspectorAgentBase is the base class for inspector agents.
type InspectorAgentBase struct {
	Name string
}

// NewInspectorAgentBase creates a new agent base.
func NewInspectorAgentBase(name string) *InspectorAgentBase {
	return &InspectorAgentBase{Name: name}
}

// InspectorAgentRegistry manages a collection of inspector agents.
type InspectorAgentRegistry struct {
	agents []interface{}
}

// NewInspectorAgentRegistry creates a new agent registry.
func NewInspectorAgentRegistry() *InspectorAgentRegistry {
	return &InspectorAgentRegistry{}
}

// AddAgent adds an agent to the registry.
func (r *InspectorAgentRegistry) AddAgent(agent interface{}) {
	r.agents = append(r.agents, agent)
}

// InspectorEnvironment provides the environment for inspector agents.
type InspectorEnvironment interface {
	SupportsModernRuntime() bool
	IsValid() bool
}

// InspectorFrontendChannel defines the interface for inspector frontend communication.
type InspectorFrontendChannel interface {
	SendMessageToFrontend(message string)
}

// InspectorFrontendRouter routes messages to frontend channels.
type InspectorFrontendRouter struct {
	channels []InspectorFrontendChannel
}

// NewInspectorFrontendRouter creates a new frontend router.
func NewInspectorFrontendRouter() *InspectorFrontendRouter {
	return &InspectorFrontendRouter{}
}

// Connect adds a frontend channel.
func (r *InspectorFrontendRouter) Connect(channel InspectorFrontendChannel) {
	r.channels = append(r.channels, channel)
}

// SendMessage sends a message to all connected frontends.
func (r *InspectorFrontendRouter) SendMessage(message string) {
	for _, ch := range r.channels {
		ch.SendMessageToFrontend(message)
	}
}

// InspectorTarget represents a debuggable target (e.g., page, worker).
type InspectorTarget interface {
	Identifier() string
	Name() string
	IsMainTarget() bool
	Connect() bool
	Disconnect()
}

// InspectorBackendDispatcher dispatches backend commands.
type InspectorBackendDispatcher struct {
	agents map[string]interface{}
}
