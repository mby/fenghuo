import type { ComponentProps } from "react";
import { Link, useLocation } from "react-router";
import { nodeDetailState } from "@/lib/node-detail-route";

export function NodeDetailLink({
  uuid,
  ...props
}: Omit<ComponentProps<typeof Link>, "to" | "state"> & { uuid: string }) {
  const location = useLocation();
  return <Link {...props} to={`/node/${encodeURIComponent(uuid)}`} state={nodeDetailState(location)} />;
}
