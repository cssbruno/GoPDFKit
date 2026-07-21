(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioAuthoringModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';

  function normalize(payload, workspace) {
    if (!payload || payload.format_version !== 1 || !workspace ||
        payload.revision !== workspace.revision || payload.plan_hash !== workspace.plan_hash ||
        payload.source_revision !== workspace.source_revision) throw new Error('Stale authoring metadata');
    return Object.freeze({
      revision: payload.revision,
      sourceRevision: payload.source_revision,
      documentTarget: payload.document_target || '',
      templateTargets: Object.freeze((payload.template_targets || []).map(Object.freeze)),
      bindingTargets: Object.freeze((payload.binding_targets || []).map(Object.freeze)),
      schemas: Object.freeze((payload.schemas || []).map(schema => Object.freeze({name: schema.name, fields: Object.freeze((schema.fields || []).map(Object.freeze))}))),
      objectTypes: Object.freeze([...(payload.object_types || [])]),
      schemaFields: Object.freeze((payload.schema_field_targets || []).map(Object.freeze)),
      scenarios: Object.freeze([...(payload.scenarios || [])]),
      scenarioValues: Object.freeze((payload.scenario_values || []).map(Object.freeze)),
      presets: Object.freeze([...(payload.stress_presets || [])]),
    });
  }

  function readableID(value) { return /^@[A-Za-z_][A-Za-z0-9_-]{0,127}$/.test(String(value || '')); }

  function buildPayload(workspace, metadata, draft) {
    if (!workspace || !metadata || metadata.revision !== workspace.revision || metadata.sourceRevision !== workspace.source_revision) throw new Error('Exact revisions are unavailable');
    const base = {source_revision: workspace.source_revision, plan_revision: workspace.revision, scenario: workspace.scenario || '', operation: draft.operation, property: ''};
    if (draft.operation === 'template') {
      const target = metadata.templateTargets.find(item => item.id === draft.target);
      const validTemplate = draft.template === 'page' ? target?.kind === 'document' : ['paragraph', 'section'].includes(draft.template) && ['body', 'row', 'column'].includes(target?.kind);
      if (!target || !validTemplate || !readableID(draft.id)) throw new Error('Choose a compatible template target, shape, and readable @id');
      return {...base, target: draft.target, template: draft.template, id: draft.id};
    }
    if (draft.operation === 'schema') {
      if (draft.target !== metadata.documentTarget || !readableID(draft.id)) throw new Error('Choose the document and a readable schema @id');
      return {...base, target: draft.target, id: draft.id};
    }
    if (draft.operation === 'schema-object') {
      if (draft.target !== metadata.documentTarget || !readableID(draft.id)) throw new Error('Choose the document and a readable custom object @id');
      return {...base, target: draft.target, id: draft.id};
    }
    if (draft.operation === 'import') {
      if (draft.target !== metadata.documentTarget || !String(draft.importPath || '').trim() || /^(?:[A-Za-z]:|[\\/]|~)|:\/\//.test(String(draft.importPath))) throw new Error('Choose the document and a safe project-relative .paper import path');
      return {...base, target: draft.target, import_path: String(draft.importPath).trim()};
    }
    if (draft.operation === 'binding') {
      const schema = metadata.schemas.find(item => item.fields.some(field => field.path === draft.path));
      if (!metadata.bindingTargets.some(item => item.id === draft.target) || !schema) throw new Error('Choose an exact node and compiler-provided binding path');
      const payload = {...base, target: draft.target, path: draft.path};
      if (draft.required !== undefined && draft.required !== '') payload.required = draft.required === true || draft.required === 'true';
      if (draft.format) payload.format = draft.format;
      if (draft.formatLocale) payload.format_locale = draft.formatLocale;
      if (draft.formatCurrency) payload.format_currency = draft.formatCurrency;
      for (const [key, value] of [['format_min_fraction', draft.minFraction], ['format_max_fraction', draft.maxFraction]]) {
        if (value !== undefined && value !== '') {
          const number = Number(value);
          if (!Number.isInteger(number) || number < 0 || number > 18) throw new Error('Binding fraction digits must be an integer from 0 through 18');
          payload[key] = number;
        }
      }
      return payload;
    }
    if (draft.operation === 'scenario-create') {
      if (!metadata.documentTarget || draft.target !== metadata.documentTarget || !metadata.schemas.some(item => item.name === draft.schema) || !metadata.presets.includes(draft.preset) || !readableID(draft.id)) throw new Error('Choose a schema, stress preset, and readable scenario @id');
      if (metadata.scenarios.includes(draft.id)) throw new Error('Scenario ID already exists');
      return {...base, target: draft.target, id: draft.id, schema: draft.schema, preset: draft.preset};
    }
    if (draft.operation === 'scenario-matrix') {
      if (!metadata.documentTarget || draft.target !== metadata.documentTarget || !metadata.schemas.some(item => item.name === draft.schema)) throw new Error('Choose a schema for the scenario matrix');
      const cases = String(draft.cases || '').split(',').map(value => value.trim()).filter(Boolean).map(value => {
        const [name, preset = 'typical'] = value.split(':').map(part => part.trim());
        return {name, preset};
      });
      if (!cases.length || cases.length > 16 || cases.some(item => !readableID(item.name) || !metadata.presets.includes(item.preset)) || new Set(cases.map(item => item.name)).size !== cases.length) throw new Error('Use unique cases such as @empty:empty,@typical:typical,@stress:stress');
      if (cases.some(item => metadata.scenarios.includes(item.name))) throw new Error('A matrix case ID already exists');
      return {...base, target: draft.target, schema: draft.schema, cases};
    }
    if (draft.operation === 'schema-field') {
      const target = metadata.schemaFields.find(item => item.id === draft.target);
      const types = ['string', 'number', 'bool', 'object', 'list', ...metadata.objectTypes];
      if (!target || !types.includes(draft.kind) || !readableID(draft.id)) throw new Error('Choose an object/schema target, field type, and readable @id');
      if (draft.kind === 'list' && !['string', 'number', 'bool', 'object', ...metadata.objectTypes].includes(draft.itemType)) throw new Error('Choose a supported list item type');
      const payload = {...base, target: draft.target, id: draft.id, kind: draft.kind};
      if (draft.kind === 'list') { payload.text = draft.itemType; payload.weight = Number(draft.maxItems || 16); }
      return payload;
    }
    if (draft.operation === 'scenario-value') {
      const choice = metadata.scenarioValues.find(item => item.scenario === draft.target && item.path === draft.path);
      if (!choice) throw new Error('Choose an exact scenario fixture value');
      if (choice.kind === 'bool' && draft.text !== 'true' && draft.text !== 'false') throw new Error('Boolean fixture values must be true or false');
      if (choice.kind === 'number' && !Number.isFinite(Number(draft.text))) throw new Error('Number fixture values must be finite');
      return {...base, target: draft.target, path: draft.path, kind: choice.kind, text: String(draft.text ?? '')};
    }
    throw new Error('Unsupported authoring operation');
  }

  function buildScenarioLifecyclePayload(workspace, metadata, draft) {
    if (!workspace || !metadata || metadata.revision !== workspace.revision || metadata.sourceRevision !== workspace.source_revision) throw new Error('Exact revisions are unavailable');
    if (!metadata.scenarios.includes(draft.target)) throw new Error('Choose an exact authored scenario');
    const base = {source_revision: workspace.source_revision, plan_revision: workspace.revision, scenario: workspace.scenario || '', operation: 'scenario', target: draft.target, property: draft.action};
    if (draft.action === 'delete') return base;
    if (draft.action === 'rename' && readableID(draft.id) && !metadata.scenarios.includes(draft.id) && draft.id !== draft.target) return {...base, id: draft.id};
    throw new Error('Choose a distinct readable scenario @id');
  }

  return Object.freeze({normalize, buildPayload, buildScenarioLifecyclePayload});
});
