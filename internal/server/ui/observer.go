package ui

import "flowk/internal/app"

type HubObserver struct {
	hub *EventHub
}

func NewHubObserver(hub *EventHub) *HubObserver {
	if hub == nil {
		return nil
	}
	return &HubObserver{hub: hub}
}

func (o *HubObserver) OnEvent(event app.FlowEvent) {
	if o == nil || o.hub == nil {
		return
	}
	o.hub.Publish(event)
}
