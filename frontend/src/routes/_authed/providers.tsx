import { createFileRoute, Outlet } from "@tanstack/react-router";

// Layout shell so /providers/$id can mount via Outlet (list lives on index).
export const Route = createFileRoute("/_authed/providers")({
  component: () => <Outlet />,
});
