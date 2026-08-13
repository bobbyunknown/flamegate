import { request } from "./client";
import type { Skill } from "./types";

export const listSkills = () => request<{ skills: Skill[] }>("GET", "/skills");

export const createSkill = (input: { name: string; description?: string; prompt?: string; enabled?: boolean }) =>
  request<Skill>("POST", "/skills", input);

export const updateSkill = (id: string, patch: { enabled?: boolean }) =>
  request<void>("POST", `/skills/${id}`, patch);

export const deleteSkill = (id: string) => request<void>("DELETE", `/skills/${id}`);
