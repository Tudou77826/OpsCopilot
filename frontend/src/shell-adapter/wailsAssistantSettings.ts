import { GetAssistantSettings, SaveAssistantSettings } from '../../wailsjs/go/main/App';
import { makeAssistantSettingsHost } from '../../../frontend-shell/src/assistant/settings';

export const wailsAssistantSettings = makeAssistantSettingsHost((method, params = {}) => {
  if (method === 'getSettings') return GetAssistantSettings();
  return SaveAssistantSettings(String(params.personaPrompt), String(params.revision));
});
