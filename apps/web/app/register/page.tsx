import { Suspense } from "react";

import { AuthForm } from "@/components/auth-form";

// Suspense boundary: the form reads ?next= from the URL, which Next requires
// to be isolated so the rest of the page can still be prerendered.
export default function RegisterPage() {
  return (
    <Suspense>
      <AuthForm mode="register" />
    </Suspense>
  );
}
