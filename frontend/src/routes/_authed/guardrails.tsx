import { createFileRoute } from "@tanstack/react-router";
import { GuardrailsPage } from "../../pages/Guardrails";

export const Route = createFileRoute("/_authed/guardrails")({
  component: GuardrailsPage,
});
