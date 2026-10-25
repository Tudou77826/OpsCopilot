import * as App from "../../wailsjs/go/main/App";
import type { LibraryHost } from "../components/AssistantLibrary/types";
import { assistantskills } from '../../wailsjs/go/models';

export const wailsAssistantLibrary: LibraryHost = {
  read: async () => {
    const state = await App.AssistantLibrary();
    return {...state, commands: state.commands.map(command => ({...command, fields: (command.fields ?? []).map(field => {
      if (field.kind !== 'text' && field.kind !== 'connection') throw Error('快捷命令包含未知输入类型');
      return {...field, kind: field.kind};
    })}))};
  },
  selectDirectory: App.SelectAssistantSkillDirectory,
  importSkill: App.ImportAssistantSkill,
  enableSkill: App.EnableAssistantSkill,
  removeSkill: App.RemoveAssistantSkill,
  saveCommand: command => App.SaveAssistantCommand(new assistantskills.Command(command)),
  removeCommand: App.RemoveAssistantCommand,
  connections: App.AssistantCommandConnections,
  prepare: App.PrepareAssistantCommand,
};
