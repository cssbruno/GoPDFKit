(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioIssueModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  function format(issue) {
    if (!issue || typeof issue !== 'object') throw new TypeError('Issue is required');
    const code = String(issue.code || issue.stage || 'PAPER_ISSUE').trim();
    const message = String(issue.message || 'Unknown issue').trim();
    const location = issue.start_line ? `Line ${issue.start_line}:${issue.start_column || 1}` : String(issue.stage || 'Unknown location');
    return `[${code}] ${message}\n${location}`;
  }

  function sourceAnnotations(issues, lineCount) {
    const count = Number(lineCount);
    if (!Number.isInteger(count) || count < 0) throw new TypeError('Source line count must be a non-negative integer');
    const grouped = new Map();
    for (const issue of Array.isArray(issues) ? issues : []) {
      const line = Number(issue?.start_line);
      if (!Number.isInteger(line) || line < 1 || line > count) continue;
      if (!grouped.has(line)) grouped.set(line, []);
      grouped.get(line).push(issue);
    }
    return [...grouped.entries()].sort((a, b) => a[0] - b[0]).map(([line, entries]) => {
      const first = entries[0];
      const code = String(first.code || first.stage || 'PAPER_ISSUE').trim();
      const message = String(first.message || 'Unknown issue').trim();
      return Object.freeze({
        line,
        severity: entries.some(issue => issue?.severity === 'error') ? 'error' : String(first.severity || 'warning'),
        label: entries.length === 1 ? `${code} · ${message}` : `${code} · ${message} · +${entries.length - 1}`,
        title: entries.map(format).join('\n\n'),
        issues: Object.freeze(entries.slice()),
      });
    });
  }

  return Object.freeze({format, sourceAnnotations});
});
