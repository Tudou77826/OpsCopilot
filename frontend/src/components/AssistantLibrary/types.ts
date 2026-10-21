export interface SkillEntry {
  name: string;
  description: string;
  instructions: string;
  enabled: boolean;
  files: string[];
}
export interface CommandField {
  key: string;
  label: string;
  kind: "text" | "connection";
  required: boolean;
}
export interface AssistantCommand {
  name: string;
  description: string;
  skill: string;
  template: string;
  fields: CommandField[];
}
export interface LibraryState {
  skills: SkillEntry[];
  commands: AssistantCommand[];
}
export interface ConnectionChoice {
  id: string;
  name: string;
}
export interface CommandDraft {
  command: AssistantCommand;
  template: string;
  values: Record<string, string>;
}
export interface LibraryHost {
  read(): Promise<LibraryState>;
  selectDirectory(): Promise<string>;
  importSkill(
    directory: string,
    replace: boolean,
    expected: string,
  ): Promise<void>;
  enableSkill(name: string, enabled: boolean): Promise<void>;
  removeSkill(name: string): Promise<void>;
  saveCommand(command: AssistantCommand): Promise<void>;
  removeCommand(name: string): Promise<void>;
  connections(): Promise<ConnectionChoice[]>;
  prepare(
    name: string,
    template: string,
    values: Record<string, string>,
  ): Promise<string>;
}
