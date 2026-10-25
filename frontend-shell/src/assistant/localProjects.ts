export interface LocalProject { id: string; name: string; directories: string[]; }
export interface LocalProjectState { version: number; activeId: string; projects: LocalProject[]; }
