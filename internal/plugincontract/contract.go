// Package plugincontract defines the public compatibility descriptor. Product
// release labels are informational; API revisions and capabilities are binding.
package plugincontract

const Marker = "OpsCopilot.TeamsPlugin.Protocol1.SharedConfig1"

func Info(version string) map[string]any {
	return map[string]any{
		"product": "OpsCopilot", "version": version, "marker": Marker,
		"protocol": 1, "apiRevision": 1, "sharedConfig": 1, "installationLifecycle": 1,
		"capabilities": []string{"connections.v1", "terminal.v1", "files.v1", "scripts.v1", "quickCommands.v1", "settings.v1"},
	}
}
