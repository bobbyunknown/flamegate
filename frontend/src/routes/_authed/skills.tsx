import { createFileRoute } from "@tanstack/react-router";
import { SkillsPage } from "../../pages/Skills";

export const Route = createFileRoute("/_authed/skills")({
  component: SkillsPage,
});
