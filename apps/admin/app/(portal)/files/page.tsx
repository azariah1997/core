import { ComingSoon } from "../../../components/ComingSoon";

export default function FilesPage() {
  return (
    <ComingSoon
      area="Files"
      reason="files only exposes GET /v1/files as listMine - there is no admin-wide object listing, and storage usage is better surfaced later via MinIO/S3 bucket metrics than by paginating individual file records here."
    />
  );
}
