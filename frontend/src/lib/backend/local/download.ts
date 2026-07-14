// downloadBlob hands an in-memory string to the browser as a file download —
// the web stand-in for the desktop app's native save dialog.
export function downloadBlob(filename: string, content: string, mime: string): void {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.style.display = 'none';
  document.body.appendChild(a);
  a.click();
  a.remove();
  // Revoke on a delay so slower engines (Safari) finish reading the blob first.
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
