import { createFileRoute } from "@tanstack/react-router";
import { KeysPage } from "../../pages/Keys";

export const Route = createFileRoute("/_authed/keys")({
  component: KeysPage,
});
