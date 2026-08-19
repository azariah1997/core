"use server";

import { revalidatePath } from "next/cache";
import { assignRole, revokeRole, ApiError } from "../../../../lib/api";

export type RoleActionState = { error?: string };

export async function assignRoleAction(userId: string, _prev: RoleActionState, formData: FormData): Promise<RoleActionState> {
  const role = String(formData.get("role") || "");
  if (!role) return { error: "Pick a role first." };
  try {
    await assignRole(userId, role);
  } catch (e) {
    return { error: e instanceof ApiError ? `${e.status}: ${e.message}` : "Failed to assign role." };
  }
  revalidatePath(`/users/${userId}`);
  return {};
}

export async function revokeRoleAction(userId: string, _prev: RoleActionState, formData: FormData): Promise<RoleActionState> {
  const role = String(formData.get("role") || "");
  if (!role) return { error: "Missing role." };
  try {
    await revokeRole(userId, role);
  } catch (e) {
    return { error: e instanceof ApiError ? `${e.status}: ${e.message}` : "Failed to revoke role." };
  }
  revalidatePath(`/users/${userId}`);
  return {};
}
