export interface AssistantSettings {
  personaPrompt: string;
  defaultPrompt: string;
  revision: string;
}
export interface AssistantSettingsHost {
  load(): Promise<AssistantSettings>;
  save(personaPrompt: string, revision: string): Promise<AssistantSettings>;
}
export function makeAssistantSettingsHost(call: (method: string, params?: Record<string, unknown>) => Promise<unknown>): AssistantSettingsHost {
  return {
    load: async () => await call('getSettings') as AssistantSettings,
    save: async (personaPrompt, revision) => await call('saveSettings', {personaPrompt, revision}) as AssistantSettings,
  };
}
