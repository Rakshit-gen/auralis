import { Spinner } from "@/components/ui";

export default function Loading() {
  return (
    <div className="container-page py-24">
      <Spinner label="Loading" />
    </div>
  );
}
