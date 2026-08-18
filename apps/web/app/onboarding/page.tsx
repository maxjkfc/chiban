import { Suspense } from "react";

import { OnboardingForm } from "@/components/onboarding-form";

// Suspense boundary: the form reads ?next= from the URL, which Next requires
// to be isolated so the rest of the page can still be prerendered.
export default function OnboardingPage() {
  return (
    <Suspense>
      <OnboardingForm />
    </Suspense>
  );
}
