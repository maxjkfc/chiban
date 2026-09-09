export const primaryNavItems = [
  { href: "/groups", label: "群組", icon: "groups", primary: false },
  { href: "/record", label: "記錄", icon: "record", primary: true },
  { href: "/profile", label: "我的", icon: "profile", primary: false },
] as const;

export function isActiveNavItem(pathname: string, href: string) {
  return pathname === href || pathname.startsWith(`${href}/`);
}