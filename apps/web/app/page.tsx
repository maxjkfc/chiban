import { redirect } from "next/navigation";

/** The app's home is Today; the gate there sorts out login and onboarding. */
export default function Home() {
  redirect("/today");
}
