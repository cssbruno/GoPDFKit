(function (root, factory) {
  const gate = factory();
  if (typeof module === 'object' && module.exports) module.exports = gate;
  else root.PaperStudioMutationGate = gate;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  function revisionsLocked(workspace, revision, sourceRevision, previewStale) {
    return Boolean(previewStale || !workspace?.revision || !workspace?.source_revision ||
      revision !== workspace.revision || sourceRevision !== workspace.source_revision);
  }

  function visualMutationsLocked(workspace, revision, sourceRevision, previewStale, committing) {
    return Boolean(committing || revisionsLocked(workspace, revision, sourceRevision, previewStale));
  }

  return Object.freeze({revisionsLocked, visualMutationsLocked});
});
