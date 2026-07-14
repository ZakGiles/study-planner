// TypeScript port of the Go .ics builder (ical.go). Renders the outstanding
// study schedule as an RFC 5545 calendar: every not-done session of every
// non-archived task becomes one all-day VEVENT. Events sort by date then UID
// so output is stable; each UID is the session id so re-imports update rather
// than duplicate in calendars that key on UID. `now` stamps DTSTAMP and is
// passed in to keep output deterministic for tests.
import { addDaysISO, isValidDate, type Task } from './models';

export const ICAL_PROD_ID = '-//study-planner//Study Planner//EN';

const pad = (n: number) => String(n).padStart(2, '0');

// icalStamp formats an instant as the RFC 5545 UTC DATE-TIME used for DTSTAMP.
function icalStamp(now: Date): string {
  return (
    `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}` +
    `T${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}Z`
  );
}

// icalDate formats a YYYY-MM-DD string as an RFC 5545 DATE value (YYYYMMDD).
const icalDate = (iso: string) => iso.replace(/-/g, '');

export function buildICS(tasks: Task[], now: Date): string {
  type Event = { uid: string; date: string; summary: string; description: string };
  const events: Event[] = [];
  for (const t of tasks) {
    if (t.archived) continue;
    for (const s of t.sessions) {
      if (s.done) continue;
      if (!isValidDate(s.date)) continue; // skip malformed dates rather than emit a broken event
      events.push({ uid: s.id, date: s.date, summary: 'Study: ' + t.name, description: t.description });
    }
  }
  events.sort((a, b) =>
    a.date < b.date ? -1 : a.date > b.date ? 1 : a.uid < b.uid ? -1 : a.uid > b.uid ? 1 : 0
  );

  const stamp = icalStamp(now);
  let out = '';
  const writeLine = (s: string) => {
    out += foldICSLine(s) + '\r\n';
  };

  writeLine('BEGIN:VCALENDAR');
  writeLine('VERSION:2.0');
  writeLine('PRODID:' + ICAL_PROD_ID);
  writeLine('CALSCALE:GREGORIAN');
  writeLine('METHOD:PUBLISH');
  writeLine('X-WR-CALNAME:Study Planner');
  for (const e of events) {
    writeLine('BEGIN:VEVENT');
    writeLine('UID:' + e.uid + '@study-planner');
    writeLine('DTSTAMP:' + stamp);
    writeLine('DTSTART;VALUE=DATE:' + icalDate(e.date));
    // DTEND is exclusive; an all-day event spans exactly one day.
    writeLine('DTEND;VALUE=DATE:' + icalDate(addDaysISO(e.date, 1)));
    writeLine('SUMMARY:' + escapeICSText(e.summary));
    if (e.description !== '') writeLine('DESCRIPTION:' + escapeICSText(e.description));
    writeLine('END:VEVENT');
  }
  writeLine('END:VCALENDAR');
  return out;
}

// escapeICSText escapes a value for an RFC 5545 TEXT field: backslash first
// (so escapes aren't double-escaped), then semicolon and comma, then CR/LF/CRLF
// become the literal "\n". Colons are not special in TEXT values.
export function escapeICSText(s: string): string {
  return s
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\r\n|\r|\n/g, '\\n');
}

const encoder = new TextEncoder();

// foldICSLine folds a content line to the RFC 5545 limit of 75 octets per
// physical line, continuing with CRLF + one leading space (which costs the
// continuation line an octet). The limit is measured in UTF-8 octets but
// folding happens on code-point boundaries so multi-byte characters never
// split — mirrors Go's rune loop.
export function foldICSLine(line: string): string {
  const limit = 75;
  if (encoder.encode(line).length <= limit) return line;
  let out = '';
  let count = 0;
  let first = true;
  for (const r of line) {
    const n = encoder.encode(r).length;
    const max = first ? limit : limit - 1;
    if (count + n > max) {
      out += '\r\n ';
      count = 0;
      first = false;
    }
    out += r;
    count += n;
  }
  return out;
}
