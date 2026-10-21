import * as App from "../../wailsjs/go/main/App";
import type { LibraryHost } from "../components/AssistantLibrary/types";

export const wailsAssistantLibrary: LibraryHost = {
  read: App.AssistantLibrary,
  selectDirectory: App.SelectAssistantSkillDirectory,
  importSkill: App.ImportAssistantSkill,
  enableSkill: App.EnableAssistantSkill,
  removeSkill: App.RemoveAssistantSkill,
  saveCommand: App.SaveAssistantCommand,
  removeCommand: App.RemoveAssistantCommand,
  connections: App.AssistantCommandConnections,
  prepare: App.PrepareAssistantCommand,
};
