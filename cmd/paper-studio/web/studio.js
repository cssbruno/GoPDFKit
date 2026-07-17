const state = {
  workspace: null,
  revision: '',
  sourceRevision: '',
  scenario: '',
  loadedScenario: '',
  page: 1,
  zoom: 1,
  zoomRender: 0,
  activePageMeta: null,
  pageMeta: new Map(),
  inspections: new Map(),
  objectURLs: new Set(),
  selectionFragments: [],
  overlays: new Set(),
  editSelection: null,
  editDraft: null,
  editFeedback: null,
  committing: false,
  resources: [],
  authoring: null,
  authoringDraft: {operation: 'template'},
  authoringFeedback: null,
  pageSetupDraft: null,
  pageSetupFeedback: null,
  loading: false,
  refreshPromise: null,
  pdfTags: null,
  pdfTagsRevision: '',
  tagsLoading: false,
  tagError: '',
  verificationStale: false,
  poll: null,
};

const $ = (selector) => document.querySelector(selector);
const app = $('#app');
const pageImage = $('#page-image');
const geometryImage = $('#geometry-image');
const inspectionLayer = $('#inspection-layer');
const overlapPicker = $('#overlap-picker');
const selectionLayer = $('#selection-layer');
const canvasScroll = $('#canvas-scroll');

function previewRevisionLocked() {
  return PaperStudioMutationGate.revisionsLocked(
    state.workspace, state.revision, state.sourceRevision, app.classList.contains('is-stale'),
  );
}

function visualMutationsLocked() {
  return PaperStudioMutationGate.visualMutationsLocked(
    state.workspace, state.revision, state.sourceRevision, app.classList.contains('is-stale'), state.committing,
  );
}

function setPreviewStale(stale) {
  app.classList.toggle('is-stale', stale);
  renderVerificationState();
  renderEditControls();
  renderAuthoringControls();
  renderPageSetup();
  renderResources();
  document.querySelectorAll('.font-replacement-apply').forEach((button) => { button.disabled = visualMutationsLocked(); });
}

async function api(path, options = {}) {
  const response = await fetch(path, {cache: 'no-store', ...options});
  const type = response.headers.get('content-type') || '';
  if (!response.ok) {
    const failure = type.includes('json') ? await response.json() : {error: await response.text()};
    const error = new Error(failure.error || `Request failed (${response.status})`);
    error.status = response.status;
    throw error;
  }
  return type.includes('json') ? response.json() : response;
}

async function refresh({quiet = false} = {}) {
  if (state.refreshPromise) {
    await state.refreshPromise;
    if (quiet) return;
  }
  if (state.refreshPromise) return state.refreshPromise;
  const pending = performRefresh({quiet});
  state.refreshPromise = pending;
  try {
    return await pending;
  } finally {
    if (state.refreshPromise === pending) state.refreshPromise = null;
  }
}

async function performRefresh({quiet = false} = {}) {
  state.loading = true;
  if (!quiet) setPreviewStale(true);
  try {
    const query = state.scenario ? `?scenario=${encodeURIComponent(state.scenario)}` : '';
    const workspace = await api(`/api/workspace${query}`);
    const workspaceScenario = workspace.scenario || '';
    const changed = workspace.revision !== state.revision || workspace.source_revision !== state.sourceRevision || workspaceScenario !== state.loadedScenario;
    state.workspace = workspace;
    state.revision = workspace.revision;
    state.sourceRevision = workspace.source_revision;
    state.scenario = workspaceScenario;
    state.loadedScenario = state.scenario;
    if (changed) {
      if (state.pdfTagsRevision && state.pdfTagsRevision !== workspace.revision) state.verificationStale = true;
      clearObjectURLs();
      state.pageMeta.clear();
      state.inspections.clear();
      state.selectionFragments = [];
      state.activePageMeta = null;
      state.pdfTags = null;
      state.pdfTagsRevision = '';
      state.tagError = '';
      closeOverlapPicker();
      state.page = Math.min(Math.max(1, state.page), Math.max(1, workspace.pages));
      renderWorkspace();
      await loadResources(workspace.revision);
      await loadAuthoring(workspace.revision);
      if (workspace.pages) await showPage(state.page);
      if (workspace.pages && app.dataset.mode === 'accessibility') await loadPDFTags();
    } else if (!quiet) {
      renderStatus();
    }
    if (!changed) await loadResources(workspace.revision);
    app.classList.toggle('has-no-plan', !workspace.pages);
    app.classList.remove('is-loading');
  } catch (error) {
    showFailure(error);
  } finally {
    state.loading = false;
    setPreviewStale(false);
  }
}

async function loadAuthoring(revision) {
  const scenario = state.scenario ? `&scenario=${encodeURIComponent(state.scenario)}` : '';
  try {
    const payload = await api(`/api/authoring?revision=${encodeURIComponent(revision)}${scenario}`);
    if (revision !== state.revision) return;
    state.authoring = PaperStudioAuthoringModel.normalize(payload, state.workspace);
  } catch (error) {
    if (error.status !== 409 && revision === state.revision) state.authoring = null;
  }
  renderAuthoringControls();
}

async function loadResources(revision) {
  const scenario = state.scenario ? `&scenario=${encodeURIComponent(state.scenario)}` : '';
  try {
    const payload = await api(`/api/resources?revision=${encodeURIComponent(revision)}&source_revision=${encodeURIComponent(state.workspace?.source_revision||'')}${scenario}`);
    if (revision !== state.revision) return;
    state.resources = PaperStudioResourceModel.normalize(payload, revision, state.workspace?.source_revision, state.workspace?.plan_hash);
  } catch (error) {
    if (error.status === 409 || revision !== state.revision) return;
    state.resources = [];
  }
  renderResources();
}

function renderResources() {
  const target=$('#resources'); target.replaceChildren(); $('#resource-count').textContent=String(state.resources.length);
  if (!state.resources.length) { const empty=document.createElement('span');empty.className='quiet';empty.textContent='No catalog assets';target.append(empty);return; }
  for (const item of state.resources) {
    const row=document.createElement('article');row.className='resource-item';
    const heading=document.createElement('div');heading.className='resource-name';const name=document.createElement('span');name.textContent=item.name;const type=document.createElement('span');type.className='resource-type';type.textContent=item.mediaType;heading.append(name,type);
    const meta=document.createElement('div');meta.className='resource-meta';meta.textContent=PaperStudioResourceModel.usageLabel(item);
    const digest=document.createElement('div');digest.className='resource-meta resource-digest';digest.textContent=`sha256 ${item.digest.slice(0,12)}…`;
    row.append(heading,meta,digest);
    const lifecycle=[];if(item.kind==='font'){lifecycle.push(`license ${item.license}`);if(item.fallback.length)lifecycle.push(`fallback ${item.fallback.join(' → ')}`);}else{if(item.defaultFocusX!==null||item.defaultFocusY!==null)lifecycle.push(`default focus ${item.defaultFocusX??'auto'},${item.defaultFocusY??'auto'}`);if(item.replaces)lifecycle.push(`replaces ${item.replaces}`);}if(lifecycle.length){const line=document.createElement('div');line.className='resource-meta resource-lifecycle';line.textContent=lifecycle.join(' · ');row.append(line);}
    for (const usage of item.usages) { const button=document.createElement('button');button.className='resource-use';button.type='button';button.textContent=`${usage.node||'anonymous'} · ${usage.decorative?'decorative':usage.alt||'missing alt'} · focus ${usage.focus_x??'auto'},${usage.focus_y??'auto'}${usage.scenario?` · ${usage.scenario}`:''}`;button.addEventListener('click',()=>{const found=walkNodes(state.workspace?.ast?.root).find(({node})=>node.id===usage.node);if(found)selectSourceNode(found.node,document.querySelector(`.outline-row[data-key="${CSS.escape(usage.node)}"]`));});row.append(button); }
    if(item.kind==='image'&&item.replaces){const previous=state.resources.find(candidate=>candidate.name===item.replaces);for(const usage of previous?.usages||[]){const replace=document.createElement('button');replace.className='resource-use resource-replace';replace.type='button';replace.disabled=visualMutationsLocked();replace.textContent=`Replace ${usage.node} with ${item.name}`;replace.addEventListener('click',()=>commitResourceReplacement(usage.node,item.name));row.append(replace);}}
    target.append(row);
  }
}

async function commitResourceReplacement(target, resource) {
  if(visualMutationsLocked()||!state.workspace)return;state.committing=true;renderResources();
  let payload;try{payload=PaperStudioResourceModel.replacementPayload(state.workspace,state.resources,target,resource);}catch(error){state.committing=false;showFailure(error);renderResources();return;}
  try{await api('/api/edit',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(payload)});await refresh();}catch(error){if(error.status===409)await refresh();else showFailure(error);}finally{state.committing=false;renderResources();}
}

function renderWorkspace() {
  const workspace = state.workspace;
  $('#file-name').textContent = workspace.file.split(/[\\/]/).pop();
  $('#revision').textContent = workspace.revision;
  $('#page-total').textContent = workspace.pages;
  renderSource(workspace.source || '', workspace.diagnostics || []);
  renderOutline(workspace.ast?.root);
  renderScenarios(workspace.ast?.root);
  renderIssues(workspace.diagnostics || []);
  renderPDFTags();
  renderThumbnails(workspace.pages, workspace.page_rail || []);
  renderBaseline(workspace.baseline || {status: 'none'});
  reconcileEditSelection();
  renderAuthoringControls();
  renderPageSetup();
  renderStatus();
}

function renderPageSetup() {
  const target = $('#page-setup-controls');
  if (!target) return;
  target.replaceChildren();
  let current;
  try {
    current = PaperStudioPageSetupModel.dimensions(state.workspace);
  } catch (error) {
    $('#page-size-summary').textContent = 'Unavailable';
    target.innerHTML = '<span class="quiet">Add one addressed page master to enable page setup.</span>';
    return;
  }
  if (!state.pageSetupDraft || state.pageSetupDraft.target !== current.target || state.pageSetupDraft.revision !== state.revision) {
    state.pageSetupDraft = {
      target: current.target, revision: state.revision, preset: current.preset, orientation: current.orientation, unit: 'mm',
      width: Number((current.width * 25.4 / 72).toFixed(2)), height: Number((current.height * 25.4 / 72).toFixed(2)),
    };
  }
  const draft = state.pageSetupDraft;
  $('#page-size-summary').textContent = `${draft.preset} · ${draft.orientation}`;
  const form = document.createElement('form');
  form.className = 'page-setup-form';
  form.addEventListener('submit', event => event.preventDefault());
  const preset = authoringSelect('Preset', [...Object.keys(PaperStudioPageSetupModel.presets), 'Custom'], draft.preset, value => {
    draft.preset = value;
    renderPageSetup();
    if (value !== 'Custom') commitPageSetup({...draft});
  });
  preset.classList.add('page-size-preset');
  form.append(preset, authoringSelect('Orientation', ['portrait', 'landscape'], draft.orientation, value => {
    draft.orientation = value;
    commitPageSetup({...draft});
  }));
  if (draft.preset === 'Custom') {
    form.append(
      pageSetupInput('Width', draft.width, value => { draft.width = value; commitPageSetup({...draft}); }),
      pageSetupInput('Height', draft.height, value => { draft.height = value; commitPageSetup({...draft}); }),
      authoringSelect('Unit', ['mm', 'in', 'pt'], draft.unit, value => {
        const points = {mm: 72 / 25.4, in: 72, pt: 1};
        draft.width = Number((Number(draft.width) * points[draft.unit] / points[value]).toFixed(3));
        draft.height = Number((Number(draft.height) * points[draft.unit] / points[value]).toFixed(3));
        draft.unit = value;
        renderPageSetup();
      }),
    );
  }
  target.append(form);
  if (state.pageSetupFeedback) {
    const feedback = document.createElement('div');
    feedback.className = `edit-feedback is-${state.pageSetupFeedback.tone}`;
    feedback.textContent = state.pageSetupFeedback.text;
    target.append(feedback);
  }
}

function pageSetupInput(labelText, value, change) {
  const field = document.createElement('label');
  field.className = 'edit-field';
  const caption = document.createElement('span');
  caption.textContent = labelText;
  const input = document.createElement('input');
  input.type = 'number';
  input.min = '0.001';
  input.max = '14400';
  input.step = '0.01';
  input.value = value;
  input.disabled = visualMutationsLocked();
  input.addEventListener('change', () => change(input.value));
  field.append(caption, input);
  return field;
}

async function commitPageSetup(draft) {
  if (visualMutationsLocked()) return;
  let payload;
  try {
    payload = PaperStudioPageSetupModel.buildPayload(state.workspace, draft);
  } catch (error) {
    state.pageSetupFeedback = {tone: 'error', text: error.message};
    renderPageSetup();
    return;
  }
  state.committing = true;
  state.pageSetupFeedback = {tone: 'working', text: 'Saving page size…'};
  renderPageSetup();
  try {
    await api('/api/edit', {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(payload)});
    state.pageSetupDraft = null;
    state.pageSetupFeedback = null;
    await refresh();
  } catch (error) {
    state.pageSetupFeedback = {tone: 'error', text: error.status === 409 ? 'Page changed · refresh and try again' : error.message};
    if (error.status === 409) await refresh();
  } finally {
    state.committing = false;
    renderPageSetup();
    renderEditControls();
  }
}

function renderAuthoringControls() {
  const target = $('#authoring-controls');
  if (!target) return;
  target.replaceChildren();
  const metadata = state.authoring;
  if (!metadata) { const quiet=document.createElement('span'); quiet.className='quiet'; quiet.textContent='Authoring metadata unavailable'; target.append(quiet); return; }
  const available = [];
  if (metadata.templateTargets.length) available.push('template');
  if (metadata.bindingTargets.length && metadata.schemas.some(schema => schema.fields.length)) available.push('binding');
  if (metadata.documentTarget && metadata.schemas.length) available.push('scenario-create');
  if (!available.length) { const quiet=document.createElement('span'); quiet.className='quiet'; quiet.textContent='Add readable IDs and a schema to enable authoring'; target.append(quiet); return; }
  const draft = state.authoringDraft;
  draft.operation = available.includes(draft.operation) ? draft.operation : available[0];
  const form=document.createElement('form'); form.className='authoring-form';
  form.append(authoringSelect('Action', available, draft.operation, value => { draft.operation=value; renderAuthoringControls(); }));
  if (draft.operation === 'template') {
    const targets=metadata.templateTargets.map(item=>item.id); draft.target=targets.includes(draft.target)?draft.target:targets[0];
    const targetKind=metadata.templateTargets.find(item=>item.id===draft.target)?.kind; const choices=targetKind==='document'?['page']:['paragraph','section'];
    draft.template=choices.includes(draft.template)?draft.template:choices[0]; draft.id=draft.id|| (draft.template==='page'?'@sheet':'@new-content');
    form.append(authoringSelect('Inside',targets,draft.target,value=>{draft.target=value;renderAuthoringControls();}),authoringSelect('Template',choices,draft.template,value=>{draft.template=value;renderAuthoringControls();}),authoringInput('Readable ID',draft.id,value=>draft.id=value));
  } else if (draft.operation === 'binding') {
    const targets=metadata.bindingTargets.map(item=>item.id); const paths=metadata.schemas.flatMap(schema=>schema.fields.map(field=>field.path));
    draft.target=targets.includes(draft.target)?draft.target:targets[0]; draft.path=paths.includes(draft.path)?draft.path:paths[0];
    form.append(authoringSelect('Node',targets,draft.target,value=>draft.target=value),authoringSelect('Schema path',paths,draft.path,value=>draft.path=value));
  } else {
    const schemas=metadata.schemas.map(item=>item.name); draft.target=metadata.documentTarget; draft.schema=schemas.includes(draft.schema)?draft.schema:schemas[0]; draft.preset=metadata.presets.includes(draft.preset)?draft.preset:'typical'; draft.id=draft.id||'@stress-case';
    form.append(authoringSelect('Schema',schemas,draft.schema,value=>draft.schema=value),authoringSelect('Matrix case',metadata.presets,draft.preset,value=>draft.preset=value),authoringInput('Scenario ID',draft.id,value=>draft.id=value));
  }
  const submit=document.createElement('button'); submit.type='submit'; submit.className='edit-commit'; submit.disabled=visualMutationsLocked(); submit.textContent=state.committing?'Committing…':'Create exact patch'; form.append(submit);
  form.addEventListener('submit',async event=>{event.preventDefault();if(visualMutationsLocked())return;let payload;try{payload=PaperStudioAuthoringModel.buildPayload(state.workspace,metadata,draft);}catch(error){state.authoringFeedback={tone:'error',text:error.message};renderAuthoringControls();return;}state.committing=true;state.authoringFeedback={tone:'working',text:'Committing against exact revisions…'};renderAuthoringControls();try{const result=await api('/api/edit',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(payload)});state.authoringFeedback={tone:'success',text:`Committed · ${result.patch_count} minimal patch${result.patch_count===1?'':'es'}`};await refresh();}catch(error){state.authoringFeedback={tone:error.status===409?'stale':'error',text:error.status===409?'Stale authoring state · refreshed without applying':error.message};if(error.status===409)await refresh();}finally{state.committing=false;renderAuthoringControls();renderEditControls();}});
  target.append(form);
  if(state.authoringFeedback){const feedback=document.createElement('div');feedback.className=`edit-feedback is-${state.authoringFeedback.tone}`;feedback.textContent=state.authoringFeedback.text;target.append(feedback);}
}

function authoringSelect(labelText, values, selected, change) {
  const field=document.createElement('label');field.className='edit-field';const caption=document.createElement('span');caption.textContent=labelText;const select=document.createElement('select');select.disabled=visualMutationsLocked();
  for(const value of values){const option=document.createElement('option');option.value=value;option.textContent=value.replaceAll('-',' ');option.selected=value===selected;select.append(option);}select.addEventListener('change',()=>change(select.value));field.append(caption,select);return field;
}

function authoringInput(labelText, value, change) {
  const field=document.createElement('label');field.className='edit-field';const caption=document.createElement('span');caption.textContent=labelText;const input=document.createElement('input');input.type='text';input.value=value;input.maxLength=128;input.disabled=visualMutationsLocked();input.addEventListener('input',()=>change(input.value));field.append(caption,input);return field;
}

function renderSource(source, issues = []) {
  const target = $('#source');
  const lines = source.split('\n');
  const annotations = new Map(PaperStudioIssueModel.sourceAnnotations(issues, lines.length).map(annotation => [annotation.line, annotation]));
  target.replaceChildren();
  lines.forEach((line, index) => {
    const lineNumber = index + 1;
    const row = document.createElement('span');
    row.className = 'source-line';
    row.dataset.line = String(lineNumber);
    const code = document.createElement('span');
    code.className = 'source-line-code';
    code.innerHTML = PaperStudioSyntaxModel.highlight(line) || '&#8203;';
    row.append(code);
    const annotation = annotations.get(lineNumber);
    if (annotation) {
      row.classList.add('has-issue', `is-${annotation.severity}`);
      const marker = document.createElement('button');
      marker.type = 'button';
      marker.className = 'source-diagnostic';
      marker.textContent = annotation.label;
      marker.title = annotation.title;
      marker.setAttribute('aria-label', `Line ${lineNumber}: ${annotation.label}`);
      marker.addEventListener('click', () => focusSourceLine(lineNumber));
      row.append(marker);
    }
    target.append(row);
  });
  $('#source-gutter').textContent = Array.from({length: lines.length}, (_, index) => index + 1).join('\n');
}

function nodeLabel(node) {
  if (node.id) return node.id;
  const value = node.value?.string_value ?? node.value?.raw;
  return value ? String(value).slice(0, 38) : node.kind;
}

function walkNodes(node, depth = 0, output = []) {
  if (!node) return output;
  output.push({node, depth});
  for (const member of node.members || []) if (member.node) walkNodes(member.node, depth + 1, output);
  return output;
}

function renderOutline(root) {
  const outline = $('#outline');
  outline.replaceChildren();
  const nodes = walkNodes(root);
  $('#node-count').textContent = `${nodes.length} nodes`;
  for (const {node, depth} of nodes) {
    const row = document.createElement('button');
    row.className = 'outline-row';
    row.setAttribute('role', 'treeitem');
    row.setAttribute('aria-selected', 'false');
    if (node.id) row.dataset.key = node.id;
    row.style.paddingLeft = `${8 + Math.min(depth, 8) * 12}px`;
    row.innerHTML = `<span class="outline-kind"></span><span class="outline-label"></span>`;
    row.querySelector('.outline-kind').textContent = node.kind;
    row.querySelector('.outline-label').textContent = nodeLabel(node);
    row.addEventListener('click', () => selectSourceNode(node, row));
    outline.append(row);
  }
}

function renderScenarios(root) {
  const scenarios = walkNodes(root).filter(({node}) => node.kind === 'scenario').map(({node}) => nodeLabel(node));
  const target = $('#scenarios');
  target.replaceChildren();
  if (!scenarios.length) {
    target.innerHTML = '<span class="quiet">No authored scenarios</span>';
    return;
  }
  for (const choice of [{label: 'Default', value: ''}, ...scenarios.map((scenario) => ({label: scenario, value: scenario}))]) {
    const item = document.createElement('button');
    item.className = 'scenario';
    item.textContent = choice.label;
    item.classList.toggle('is-active', normalizeScenario(choice.value) === normalizeScenario(state.scenario));
    item.setAttribute('aria-pressed', String(normalizeScenario(choice.value) === normalizeScenario(state.scenario)));
    item.addEventListener('click', async () => {
      if (normalizeScenario(choice.value) === normalizeScenario(state.scenario)) return;
      state.scenario = choice.value;
      await refresh();
    });
    target.append(item);
  }
}

function normalizeScenario(value) {
  return String(value || '').trim().replace(/^@/, '');
}

function renderIssues(issues) {
  $('#issue-count').textContent = issues.length;
  const target = $('#issues');
  target.replaceChildren();
  if (!issues.length) {
    target.innerHTML = '<div class="inspector-empty">No current plan diagnostics.</div>';
    return;
  }
  for (const issue of issues) {
    const item = document.createElement('div');
    item.className = 'issue';
    if (issue.start_line) item.dataset.line = String(issue.start_line);
    const focus = document.createElement('button');
    focus.type = 'button';
    focus.className = 'issue-focus';
    focus.innerHTML = '<span class="issue-code"></span><span class="issue-message"></span><span class="issue-location"></span>';
    focus.querySelector('.issue-code').textContent = issue.code || issue.stage;
    focus.querySelector('.issue-message').textContent = issue.message;
    focus.querySelector('.issue-location').textContent = issue.start_line ? `Line ${issue.start_line}:${issue.start_column || 1}` : issue.stage;
    focus.addEventListener('click', () => focusSourceLine(issue.start_line || 1));
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'issue-copy';
    copy.textContent = 'Copy';
    copy.setAttribute('aria-label', `Copy ${issue.code || issue.stage || 'issue'} error`);
    copy.addEventListener('click', async () => {
      try {
        await navigator.clipboard.writeText(PaperStudioIssueModel.format(issue));
        copy.textContent = 'Copied';
        copy.dataset.status = 'copied';
        setTimeout(() => { copy.textContent = 'Copy'; delete copy.dataset.status; }, 1600);
      } catch (_) {
        copy.textContent = 'Copy failed';
        copy.dataset.status = 'failed';
      }
    });
    item.append(focus, copy);
    if (issue.code === 'PAPER_COMPILE_FONT') {
      const selection = PaperStudioEditModel.findTextSelectionAtLine(state.workspace?.ast?.root, issue.start_line);
      if (selection) item.append(fontReplacementOffer(selection));
    }
    target.append(item);
  }
}

async function loadPDFTags() {
  const workspace = state.workspace;
  if (!workspace?.pages || state.tagsLoading || state.pdfTagsRevision === workspace.revision) return;
  const revision = workspace.revision;
  state.tagsLoading = true;
  state.tagError = '';
  renderPDFTags();
  try {
    const scenario = state.scenario ? `&scenario=${encodeURIComponent(state.scenario)}` : '';
    const payload = await api(`/api/pdf-tags?revision=${encodeURIComponent(revision)}${scenario}`);
    if (revision !== state.revision) return;
    state.pdfTags = PaperStudioTagModel.normalize(payload, state.workspace);
    state.pdfTagsRevision = revision;
    state.verificationStale = false;
  } catch (error) {
    if (revision === state.revision) state.tagError = error.status === 409 ? 'Plan changed before tag inspection completed.' : error.message;
  } finally {
    state.tagsLoading = false;
    renderPDFTags();
  }
}

function renderPDFTags() {
  const status = $('#tag-status');
  const summary = $('#tag-summary');
  const tree = $('#tag-tree');
  if (!status || !summary || !tree) return;
  tree.replaceChildren();
  if (state.tagsLoading) {
    status.textContent = 'Inspecting…';
    status.dataset.status = 'loading';
    summary.textContent = 'Serializing and independently inspecting exact final PDF bytes.';
    renderVerificationState();
    return;
  }
  if (state.tagError) {
    status.textContent = 'Inspection failed';
    status.dataset.status = 'failed';
    summary.textContent = state.tagError;
    renderVerificationState();
    return;
  }
  const tags = state.pdfTags;
  if (!tags || state.pdfTagsRevision !== state.revision) {
    status.textContent = state.workspace?.pages ? 'Not inspected' : 'Unavailable';
    status.dataset.status = 'none';
    summary.textContent = state.workspace?.pages ? 'Open Accessibility to inspect the serialized PDF.' : 'A current plan is required.';
    renderVerificationState();
    return;
  }
  status.textContent = tags.passed ? 'Tags verified' : 'Invalid tags';
  status.dataset.status = tags.passed ? 'passed' : 'failed';
  summary.textContent = tags.passed
    ? `${tags.structureElements} structure elements · ${tags.contentMarked} marked streams · PDF ${tags.hash.slice(0, 12)}`
    : tags.failures.join(' · ') || 'Final PDF tag verification failed.';
  for (const node of tags.rows) {
    const row = document.createElement('div');
    row.className = 'tag-node';
    row.setAttribute('role', 'treeitem');
    row.setAttribute('aria-level', String(node.depth + 1));
    row.style.paddingLeft = `${10 + Math.min(node.depth, 12) * 12}px`;
    const role = document.createElement('span');
    role.className = 'tag-role';
    role.textContent = node.role;
    const evidence = document.createElement('span');
    evidence.className = 'tag-evidence';
    const flags = [node.markedContent ? `${node.markedContent} MCID` : '', node.hasAlt ? 'Alt' : '', node.hasActualText ? 'ActualText' : '', node.hasLanguage ? 'Lang' : ''].filter(Boolean);
    evidence.textContent = flags.join(' · ') || `${node.children} children`;
    row.append(role, evidence);
    tree.append(row);
  }
  renderVerificationState();
}

function fontReplacementOffer(selection) {
  const controls = document.createElement('div');
  controls.className = 'font-replacement';
  const select = document.createElement('select');
  select.setAttribute('aria-label', `Replacement font for ${selection.target}`);
  for (const font of PaperStudioEditModel.coreFonts) select.append(new Option(font, font));
  const apply = document.createElement('button');
  apply.type = 'button';
  apply.className = 'font-replacement-apply';
  apply.textContent = 'Replace font';
  apply.disabled = visualMutationsLocked();
  const status = document.createElement('span');
  status.className = 'font-replacement-status';
  apply.addEventListener('click', async () => {
    if (visualMutationsLocked()) return;
    let payload;
    try {
      payload = PaperStudioEditModel.buildPayload(state.workspace, selection, 'text', 'font', select.value);
    } catch (error) {
      status.textContent = error.message;
      return;
    }
    state.committing = true;
    apply.disabled = true;
    status.textContent = 'Applying exact patch…';
    try {
      await api('/api/edit', {method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(payload)});
      await refresh();
    } catch (error) {
      status.textContent = error.status === 409 ? 'Source changed; refresh and choose again.' : error.message;
    } finally {
      state.committing = false;
      apply.disabled = visualMutationsLocked();
      renderEditControls();
      renderAuthoringControls();
      renderResources();
    }
  });
  controls.append(select, apply, status);
  return controls;
}

function renderBaseline(baseline) {
  const target = $('#baseline-state');
  target.textContent = PaperStudioRailModel.baselineLabel(baseline);
  target.title = baseline.revision
    ? `Baseline ${baseline.revision.slice(0, 12)} · ${baseline.scenario || 'default'} · ${baseline.status.replaceAll('_', ' ')}`
    : target.textContent;
  target.dataset.status = baseline.status || 'none';
}

function renderThumbnails(count, summaries) {
  const target = $('#thumbnails');
  target.replaceChildren();
  const byPage = PaperStudioRailModel.pageSummaryMap(summaries);
  for (let page = 1; page <= count; page++) {
    const summary = byPage.get(page) || PaperStudioRailModel.fallbackPageSummary(page);
    const item = document.createElement('div');
    item.className = `thumbnail${page === state.page ? ' is-active' : ''}${summary.changed ? ' is-changed' : ''}`;
    const button = document.createElement('button');
    button.className = 'thumbnail-page';
    button.dataset.page = page;
    if (page === state.page) button.setAttribute('aria-current', 'page');
    button.setAttribute('aria-label', `Page ${page}, ${summary.selector} selector, ${(summary.regions || []).join(', ') || 'no retained regions'}`);
    button.innerHTML = `<span class="thumbnail-sheet"><canvas role="img" aria-label="Page ${page} WASM thumbnail"></canvas></span><span class="thumbnail-label">${page}</span>`;
    button.addEventListener('click', () => showPage(page));
    item.append(button);

    const stateLine = document.createElement('div');
    stateLine.className = 'thumbnail-state';
    const selector = document.createElement('span');
    selector.className = 'master-state';
    selector.textContent = summary.selector;
    selector.title = `${summary.selector} page-master selector`;
    stateLine.append(selector);
    for (const region of summary.regions || []) {
      const label = document.createElement('span');
      label.className = `region-state${(summary.repeated_regions || []).includes(region) ? ' is-repeated' : ''}`;
      label.textContent = region.slice(0, 1).toUpperCase();
      label.title = `${region} region${(summary.repeated_regions || []).includes(region) ? ' · repeated master content' : ''}`;
      stateLine.append(label);
    }
    item.append(stateLine);

    const badges = document.createElement('div');
    badges.className = 'thumbnail-badges';
    if (summary.changed) {
      const changed = document.createElement('button');
      changed.className = 'rail-badge is-change';
      changed.textContent = summary.change_kind === 'added' ? '+' : 'Δ';
      changed.title = `Page ${page} ${summary.change_kind} from exact baseline`;
      changed.setAttribute('aria-label', changed.title);
      changed.addEventListener('click', async () => {
        await showPage(page);
        renderInspector({plan_revision: state.revision, page, baseline: state.workspace.baseline, change_kind: summary.change_kind, content_hash: summary.content_hash}, 'Changed page');
      });
      badges.append(changed);
    }
    for (const issue of summary.issues || []) {
      const badge = document.createElement('button');
      badge.className = `rail-badge is-issue is-${issue.severity || 'warning'}`;
      badge.textContent = '!';
      badge.title = `${issue.code}: ${issue.message}`;
      badge.setAttribute('aria-label', `Page ${page} issue ${badge.title}`);
      badge.addEventListener('click', () => selectRailIssue(summary, issue));
      badges.append(badge);
    }
    item.append(badges);
    target.append(item);
    loadWASMPage(page, 72).then((rendered) => paintWASMCanvas(button.querySelector('canvas'), rendered)).catch(() => {});
  }
}

async function selectRailIssue(summary, issue) {
  const revision = state.revision;
  await showPage(summary.page);
  if (revision !== state.revision) return;
  if (issue.start_line) focusSourceLine(issue.start_line);
  let causal = null;
  const selector = issue.key ? {key: issue.key} : issue.fragment ? {fragment: issue.fragment} : null;
  if (selector) {
    try {
      causal = await api('/api/explain', {
        method: 'POST', headers: {'content-type': 'application/json'},
        body: JSON.stringify({revision, scenario: state.scenario, selector}),
      });
      if (revision !== state.revision || causal.plan_hash !== revision) return;
      state.selectionFragments = causal.targets?.[0]?.fragments || [];
    } catch (error) {
      if (error.status === 409) return refresh();
    }
  } else if (issue.has_bounds && issue.bounds?.length === 4) {
    const [x, y, width, height] = issue.bounds;
    state.selectionFragments = [{page: summary.page, border_box: {x, y, width, height}}];
  }
  renderSelectionRects({reveal: true});
  renderInspector({plan_revision: revision, page: summary.page, issue, causal}, 'Plan issue');
}

async function loadSVG(page, kind) {
  const key = `${state.revision}:${kind}:${page}`;
  if (state.pageMeta.has(key)) return state.pageMeta.get(key);
  const suffix = kind === 'geometry' ? '.geometry.svg' : '.svg';
  const scenario = state.scenario ? `&scenario=${encodeURIComponent(state.scenario)}` : '';
  const response = await api(`/api/page/${page}${suffix}?revision=${encodeURIComponent(state.revision)}${scenario}`);
  const text = await response.text();
  const match = text.match(/viewBox="([^\"]+)"/);
  const viewBox = match ? match[1].trim().split(/\s+/).map(Number) : [0, 0, 1, 1];
  const blob = new Blob([text], {type: 'image/svg+xml'});
  const url = URL.createObjectURL(blob);
  state.objectURLs.add(url);
  const result = {url, viewBox, text};
  state.pageMeta.set(key, result);
  return result;
}

async function loadWASMPage(page, dpi = renderDPIForZoom()) {
  const key = `${state.revision}:wasm:${page}:${dpi}`;
  if (state.pageMeta.has(key)) return state.pageMeta.get(key);
  const revision = state.revision;
  const scenario = state.scenario ? `&scenario=${encodeURIComponent(state.scenario)}` : '';
  const response = await api(`/api/page/${page}.render?revision=${encodeURIComponent(revision)}&dpi=${dpi}${scenario}`);
  const result = await PaperStudioWASMRenderer.renderResponse(response, {revision, page, dpi});
  if (revision !== state.revision) {
    result.bitmap.close();
    const error = new Error('WASM page belongs to a stale plan revision');
    error.status = 409;
    throw error;
  }
  state.pageMeta.set(key, result);
  return result;
}

function paintWASMCanvas(canvas, rendered) {
  canvas.width = rendered.pixelWidth;
  canvas.height = rendered.pixelHeight;
  const context = canvas.getContext('2d', {alpha: false});
  context.imageSmoothingEnabled = false;
  context.clearRect(0, 0, canvas.width, canvas.height);
  context.drawImage(rendered.bitmap, 0, 0);
}

function paintWASMPage(rendered) {
  paintWASMCanvas(pageImage, rendered);
  state.activePageMeta = rendered;
  applyPageStageWidth(rendered);
}

async function loadInspection(page) {
  const key = `${state.revision}:${page}`;
  if (state.inspections.has(key)) return state.inspections.get(key);
  const revision = state.revision;
  const inspection = await api('/api/inspect', {
    method: 'POST', headers: {'content-type': 'application/json'},
    body: JSON.stringify({revision, scenario: state.scenario, page}),
  });
  if (revision !== state.revision || inspection.plan_hash !== revision) {
    const error = new Error('Inspection belongs to a stale plan revision');
    error.status = 409;
    throw error;
  }
  state.inspections.set(key, inspection);
  return inspection;
}

async function showPage(page) {
  if (!state.workspace?.pages || page < 1 || page > state.workspace.pages) return;
  const revision = state.revision;
  if (page !== state.page) {
    selectionLayer.replaceChildren();
    canvasScroll.scrollTop = 0;
    canvasScroll.scrollLeft = 0;
  }
  setPreviewStale(true);
  try {
    const [display, geometry] = await Promise.all([loadWASMPage(page, renderDPIForZoom()), loadSVG(page, 'geometry'), loadInspection(page)]);
    if (revision !== state.revision) return;
    state.page = page;
    paintWASMPage(display);
    geometryImage.src = geometry.url;
    $('#page-label').textContent = `Page ${page} of ${state.workspace.pages}`;
    document.querySelectorAll('.thumbnail-page').forEach((button) => {
      const active = Number(button.dataset.page) === page;
      button.closest('.thumbnail')?.classList.toggle('is-active', active);
      if (active) button.setAttribute('aria-current', 'page'); else button.removeAttribute('aria-current');
    });
    renderSelectionRects();
    renderInspectionOverlays();
    closeOverlapPicker();
    renderStatus();
  } catch (error) {
    if (error.status === 409) await refresh(); else showFailure(error);
  } finally {
    setPreviewStale(state.loading);
  }
}

function inspectionTarget() {
  return state.inspections.get(`${state.revision}:${state.page}`)?.targets?.[0] || null;
}

function addInspectionRect(rect, className, label = '') {
  const meta = state.activePageMeta;
  if (!meta || !rect || !(rect.width >= 0 && rect.height >= 0)) return null;
  const [viewX, viewY, viewWidth, viewHeight] = meta.viewBox;
  if (!(viewWidth > 0 && viewHeight > 0)) return null;
  const mark = document.createElement('div');
  mark.className = `inspection-mark ${className}`;
  mark.style.left = `${((rect.x - viewX) / viewWidth) * 100}%`;
  mark.style.top = `${((rect.y - viewY) / viewHeight) * 100}%`;
  mark.style.width = `${(rect.width / viewWidth) * 100}%`;
  mark.style.height = `${(rect.height / viewHeight) * 100}%`;
  if (label) {
    const tag = document.createElement('span');
    tag.textContent = label;
    mark.append(tag);
  }
  inspectionLayer.append(mark);
  return mark;
}

function renderInspectionOverlays() {
  inspectionLayer.replaceChildren();
  const target = inspectionTarget();
  if (!target || !state.overlays.size) return;
  const fragments = [...(target.fragments || []), ...(target.continuation_fragments || [])]
    .filter((fragment, index, all) => fragment.page === state.page && all.findIndex((entry) => entry.id === fragment.id) === index);
  const fragmentByID = new Map(fragments.map((fragment) => [fragment.id, fragment]));
  const classifiedInstances = globalThis.PaperStudioInstanceModel?.classifyFragments(fragments) || [];
  const instanceByFragment = new Map(classifiedInstances.map((entry) => [entry.fragment.id, entry]));
  const semantics = target.semantics || [];
  const semanticByID = new Map(semantics.map((entry) => [entry.Node?.id ?? entry.node?.id, entry.Node ?? entry.node]));
  const semanticByIdentity = new Map(semantics.map((entry) => {
    const node = entry.Node ?? entry.node;
    return [`${node?.key || ''}\u0000${node?.instance || ''}`, node];
  }));
  const reading = (target.reading_order || []).map((entry) => entry.Occurrence ?? entry.occurrence).filter(Boolean);
  const readingByFragment = new Map(reading.map((entry) => [entry.fragment, entry]));

  const boxModels = globalThis.PaperStudioInspectionModel?.boxModelMarks(fragments, state.page) || [];
  for (const box of boxModels) {
    if (state.overlays.has('margin')) addInspectionRect(box.margin, 'is-margin');
    if (state.overlays.has('border')) addInspectionRect(box.border, 'is-border');
    if (state.overlays.has('padding')) addInspectionRect(box.padding, 'is-padding');
    if (state.overlays.has('content')) addInspectionRect(box.content, 'is-content');
  }

  for (const fragment of fragments) {
    if (state.overlays.has('instances')) {
      const instance = instanceByFragment.get(fragment.id);
      if (instance) addInspectionRect(fragment.border_box, instance.className, instance.label);
    }
    if (state.overlays.has('reading')) {
      const occurrence = readingByFragment.get(fragment.id);
      if (occurrence) addInspectionRect(fragment.border_box, 'is-reading', String(occurrence.reading_index + 1));
    }
    if (state.overlays.has('roles')) {
      const occurrence = readingByFragment.get(fragment.id);
      const source = fragment.source_identity || {};
      const semantic = semanticByID.get(occurrence?.semantic) || semanticByIdentity.get(`${source.key || ''}\u0000${source.instance || ''}`);
      if (semantic?.role) addInspectionRect(fragment.border_box, `is-role is-role-${semantic.role}`, semantic.role);
    }
  }
  if (state.overlays.has('regions')) {
    const regions = globalThis.PaperStudioInspectionModel?.pageRegionMarks(target.page_regions || [], state.page) || [];
    for (const region of regions) addInspectionRect(region.rect, `is-region is-region-${region.region}`, region.label);
  }
  if (state.overlays.has('baselines')) {
    const baselines = globalThis.PaperStudioInspectionModel?.baselineMarks(target.lines || [], state.page) || [];
    for (const baseline of baselines) addInspectionRect(baseline.rect, 'is-baseline', baseline.label);
  }
  if (state.overlays.has('tracks')) {
    const tracks = globalThis.PaperStudioInspectionModel?.gridTrackMarks(target.grid_tracks || [], state.page) || [];
    for (const track of tracks) addInspectionRect(track.rect, `is-grid-track is-grid-track-${track.axis}`, track.label);
  }
  if (state.overlays.has('cells')) {
    const cells = globalThis.PaperStudioInspectionModel?.tableCellMarks(fragments, state.page) || [];
    for (const cell of cells) addInspectionRect(cell.rect, `is-table-cell${cell.tableHeader ? ' is-table-header-cell' : ''}`, cell.label);
  }
  const issues = globalThis.PaperStudioInspectionModel?.issueMarks(target, state.page) || {};
  if (state.overlays.has('overflow')) for (const mark of issues.overflow || []) addInspectionRect(mark.rect, 'is-overflow', mark.label);
  if (state.overlays.has('clips')) for (const mark of issues.clips || []) addInspectionRect(mark.rect, 'is-clip', mark.label);
  if (state.overlays.has('collisions')) for (const mark of issues.collisions || []) addInspectionRect(mark.rect, 'is-collision', mark.label);
  if (state.overlays.has('breaks')) {
    for (const entry of target.breaks || []) {
      const decision = entry.decision || {};
      const fragment = fragmentByID.get(decision.triggering_fragment) || fragmentByID.get(decision.preceding_fragment);
      if (fragment) addInspectionRect(fragment.border_box, 'is-break', `${decision.from_page}→${decision.to_page} · ${String(decision.reason || 'break').replaceAll('_', ' ')}`);
    }
  }
}

function renderPageInspectionEvidence() {
  const target = inspectionTarget();
  if (!target) return;
  const pageRegions = (globalThis.PaperStudioInspectionModel?.pageRegionMarks(target.page_regions || [], state.page) || []).map((entry) => ({
    region: entry.region, master: entry.master, bounds: entry.rect,
  }));
  const regions = pageRegions.map((entry) => entry.region);
  const fragments = [...(target.fragments || []), ...(target.continuation_fragments || [])]
    .filter((fragment, index, all) => all.findIndex((entry) => entry.id === fragment.id) === index);
  const fragmentInstances = (globalThis.PaperStudioInstanceModel?.classifyFragments(fragments) || []).map((entry) => ({
    id: entry.fragment.id,
    key: entry.key,
    instance: entry.instance,
    region: entry.region,
    kind: entry.kind,
    repeated: entry.repeated,
  }));
  const tableCells = (globalThis.PaperStudioInspectionModel?.tableCellMarks(fragments, state.page) || []).map((entry) => ({
    semantic: entry.cell,
    fragment: entry.fragment,
    table_header: entry.tableHeader,
  }));
  const gridTracks = (globalThis.PaperStudioInspectionModel?.gridTrackMarks(target.grid_tracks || [], state.page) || []).map((entry) => ({
    group: entry.group,
    axis: entry.axis,
    index: entry.trackIndex,
    gap_after: entry.gapAfter,
    bounds: entry.rect,
  }));
  const repeated = fragmentInstances.filter(entry => entry.repeated).length;
  const tableSummary = tableCells.length || gridTracks.length ? `${tableCells.length} cells · ${gridTracks.length} tracks` : 'None';
  const provenance = globalThis.PaperStudioProvenanceModel?.forFragments(target.provenance, fragments) || {bindings: [], styleTokens: []};
  renderInspectorRows([
    ['Page', `${state.page} of ${state.workspace?.pages || state.page}`],
    ['Regions', regions.join(', ') || 'Body'],
    ['Repeated', repeated ? `${repeated} instances` : 'None'],
    ['Tables', tableSummary],
    ['Bindings', provenance.bindings.length ? `${provenance.bindings.length} paths` : 'None'],
    ['Style tokens', provenance.styleTokens.length ? `${provenance.styleTokens.length} properties` : 'None'],
    ['Reading', `${(target.reading_order || []).length} entries`],
    ['Overlays', state.overlays.size ? [...state.overlays].join(', ') : 'None'],
  ], 'Page');
  renderProvenance({provenance: target.provenance, fragments});
}

function renderSelectionRects({reveal = false} = {}) {
  const meta = state.activePageMeta;
  selectionLayer.replaceChildren();
  const selectedPages = new Set(state.selectionFragments.map((fragment) => fragment.page));
  document.querySelectorAll('.thumbnail-page').forEach((button) => button.closest('.thumbnail')?.classList.toggle('has-selection', selectedPages.has(Number(button.dataset.page))));
  if (!meta) return;
  const [viewX, viewY, viewWidth, viewHeight] = meta.viewBox;
  if (!(viewWidth > 0 && viewHeight > 0)) return;
  let first;
  for (const fragment of state.selectionFragments.filter((entry) => entry.page === state.page)) {
    const rect = fragment.border_box || fragment.content_box;
    if (!rect || rect.width < 0 || rect.height < 0) continue;
    const box = document.createElement('div');
    box.className = 'selection-box';
    box.style.left = `${((rect.x - viewX) / viewWidth) * 100}%`;
    box.style.top = `${((rect.y - viewY) / viewHeight) * 100}%`;
    box.style.width = `${(rect.width / viewWidth) * 100}%`;
    box.style.height = `${(rect.height / viewHeight) * 100}%`;
    selectionLayer.append(box);
    first ||= box;
  }
  if (reveal && first) first.scrollIntoView({block: 'center', inline: 'center'});
}

async function hitPage(event) {
  if (previewRevisionLocked()) return;
  const revision = state.revision;
  const meta = state.activePageMeta;
  if (!meta) return;
  const bounds = pageImage.getBoundingClientRect();
  const [x, y, width, height] = meta.viewBox;
  const xFixed = Math.round(x + ((event.clientX - bounds.left) / bounds.width) * width);
  const yFixed = Math.round(y + ((event.clientY - bounds.top) / bounds.height) * height);
  const pulse = $('#selection-pulse');
  pulse.style.left = `${event.clientX - bounds.left}px`;
  pulse.style.top = `${event.clientY - bounds.top}px`;
  pulse.classList.remove('is-visible');
  requestAnimationFrame(() => pulse.classList.add('is-visible'));
  try {
    const result = await api('/api/hit', {
      method: 'POST', headers: {'content-type': 'application/json'},
      body: JSON.stringify({revision: state.revision, scenario: state.scenario, page: state.page, x_fixed: xFixed, y_fixed: yFixed}),
    });
    if (revision !== state.revision || previewRevisionLocked()) return;
    const fragments = result.Fragments || [];
    const fragment = fragments[0];
    if (fragments.length > 1) openOverlapPicker(result, event.clientX - bounds.left, event.clientY - bounds.top);
    else closeOverlapPicker();
    await selectHitFragment(result, fragment);
  } catch (error) {
    if (error.status === 409) await refresh(); else showFailure(error);
  }
}

async function selectHitFragment(result, fragment) {
  const revision = state.revision;
  try {
    if (!fragment?.Key) {
      state.selectionFragments = fragment ? [{page: state.page, border_box: fragment.BorderBox, content_box: fragment.ContentBox}] : [];
      renderSelectionRects();
      renderInspector(result, 'Pixel trace');
      return;
    }
    selectEditableTarget(fragment.Key);
    markOutlineKey(fragment.Key);
    focusSourceLine(fragment.Source?.start?.line || 1);
    const explanation = await api('/api/explain', {
      method: 'POST', headers: {'content-type': 'application/json'},
      body: JSON.stringify({revision: state.revision, scenario: state.scenario, selector: {key: fragment.Key}}),
    });
    if (revision !== state.revision || previewRevisionLocked()) return;
    state.selectionFragments = explanation.targets?.[0]?.fragments || [];
    renderSelectionRects({reveal: true});
    renderInspector({causal: explanation, hit: result}, 'Pixel trace');
  } catch (error) {
    if (error.status === 409) await refresh(); else showFailure(error);
  }
}

function openOverlapPicker(result, left, top) {
  overlapPicker.replaceChildren();
  const heading = document.createElement('div');
  heading.className = 'overlap-heading';
  heading.textContent = `${result.FragmentMatchCount || result.Fragments.length} overlaps · topmost first`;
  overlapPicker.append(heading);
  result.Fragments.forEach((fragment, index) => {
    const choice = document.createElement('button');
    choice.setAttribute('role', 'option');
    choice.setAttribute('aria-selected', String(index === 0));
    choice.innerHTML = '<span class="overlap-order"></span><span class="overlap-name"></span><span class="overlap-region"></span>';
    choice.querySelector('.overlap-order').textContent = String(index + 1);
    choice.querySelector('.overlap-name').textContent = fragment.Key || fragment.Instance || `Fragment ${fragment.ID}`;
    choice.querySelector('.overlap-region').textContent = fragment.Region || 'page';
    choice.addEventListener('click', async (event) => {
      event.stopPropagation();
      overlapPicker.querySelectorAll('[role="option"]').forEach((item) => item.setAttribute('aria-selected', String(item === choice)));
      await selectHitFragment(result, fragment);
    });
    overlapPicker.append(choice);
  });
  overlapPicker.style.left = `${Math.min(Math.max(8, left + 12), Math.max(8, pageImage.clientWidth - 202))}px`;
  overlapPicker.style.top = `${Math.max(8, top + 12)}px`;
  overlapPicker.hidden = false;
  overlapPicker.querySelector('[role="option"]')?.focus({preventScroll: true});
}

function closeOverlapPicker() {
  overlapPicker.hidden = true;
  overlapPicker.replaceChildren();
}

function markOutlineKey(key) {
  document.querySelectorAll('.outline-row').forEach((item) => {
    const selected = item.dataset.key === key;
    item.classList.toggle('is-selected', selected);
    item.setAttribute('aria-selected', String(selected));
  });
}

async function selectSourceNode(node, row) {
  selectEditableTarget(node.id || '');
  if (node.id) markOutlineKey(node.id);
  else document.querySelectorAll('.outline-row').forEach((item) => {
    const selected = item === row;
    item.classList.toggle('is-selected', selected);
    item.setAttribute('aria-selected', String(selected));
  });
  focusSourceLine(node.header_span?.start?.line || node.span?.start?.line || 1);
  const selector = node.id ? {key: node.id} : {};
  if (!Object.keys(selector).length) {
    renderInspector(node, 'Source node');
    return;
  }
  try {
    const explanation = await api('/api/explain', {
      method: 'POST', headers: {'content-type': 'application/json'},
      body: JSON.stringify({revision: state.revision, scenario: state.scenario, selector}),
    });
    const fragments = explanation.targets?.[0]?.fragments || [];
    state.selectionFragments = fragments;
    const fragment = fragments[0];
    if (fragment?.page) {
      await showPage(fragment.page);
      renderSelectionRects({reveal: true});
    }
    renderInspector(explanation, 'Causal trace');
  } catch (error) {
    renderInspector(node, 'Source node');
  }
}

function renderInspector(value, kind) {
  const useful = /(key|kind|role|page|region|reason|message|source|instance|operation|property|value)$/i;
  const blocked = /(revision|hash|bounds|box|fixed|geometry|selection|content_hash)/i;
  const seen = new Set();
  const rows = flatten(value)
    .filter(([key, entry]) => !blocked.test(key) && useful.test(key) && entry !== '' && entry !== null && entry !== undefined)
    .map(([key, entry]) => [key.split('.').at(-1).replaceAll('_', ' '), entry])
    .filter(([key]) => !seen.has(key) && seen.add(key))
    .slice(0, 10);
  renderInspectorRows(rows, kind);
  const target = value?.causal?.targets?.[0] || value?.targets?.[0] || value;
  renderProvenance({provenance: target?.provenance || value?.provenance, fragments: target?.fragments || []});
}

function renderProvenance(value) {
  const target = $('#inspector-content');
  const model = globalThis.PaperStudioProvenanceModel;
  if (!model) return;
  const selected = model.forFragments(value?.provenance, value?.fragments || []);
  if (!selected.bindings.length && !selected.styleTokens.length) return;
  const section = document.createElement('section');
  section.className = 'provenance-evidence';
  const heading = document.createElement('div');
  heading.className = 'provenance-heading';
  heading.textContent = 'Source provenance';
  section.append(heading);
  for (const binding of selected.bindings) {
    const item = document.createElement('div');
    item.className = 'provenance-item';
    item.textContent = `Data · ${model.bindingLabel(binding)}`;
    section.append(item);
  }
  for (const token of selected.styleTokens) {
    const item = document.createElement('div');
    item.className = 'provenance-item';
    item.textContent = `Token · ${model.tokenLabel(token)}`;
    section.append(item);
  }
  target.append(section);
}

function renderInspectorRows(rows, kind) {
  $('#selection-kind').textContent = kind;
  const target = $('#inspector-content');
  target.replaceChildren();
  if (!rows.length) {
    target.innerHTML = '<div class="inspector-empty">Select the page or an outline item to inspect it.</div>';
    return;
  }
  for (const [key, entry] of rows) {
    const row = document.createElement('div');
    row.className = 'property';
    row.innerHTML = '<span class="property-key"></span><span class="property-value"></span>';
    row.querySelector('.property-key').textContent = key;
    row.querySelector('.property-value').textContent = String(entry);
    target.append(row);
  }
  $('#cursor-state').textContent = `Page ${state.page} · ${kind}`;
}

function selectEditableTarget(target) {
  state.editSelection = target ? PaperStudioEditModel.findSelection(state.workspace?.ast?.root, target) : null;
  state.editDraft = null;
  state.editFeedback = null;
  renderEditControls();
}

function reconcileEditSelection() {
  if (state.editSelection?.target) {
    state.editSelection = PaperStudioEditModel.findSelection(state.workspace?.ast?.root, state.editSelection.target);
  }
  renderEditControls();
}

function renderEditControls() {
  const target = $('#edit-controls');
  target.replaceChildren();
  const operations = PaperStudioEditModel.operations(state.editSelection);
  if (!operations.length) {
    target.hidden = true;
    return;
  }
  target.hidden = false;
  const operation = operations.includes(state.editDraft?.operation) ? state.editDraft.operation : operations[0];
  const availableProperties = PaperStudioEditModel.properties(state.editSelection, operation);
  const property = availableProperties.includes(state.editDraft?.property) ? state.editDraft.property : availableProperties[0];
  state.editDraft = {operation, property};

  const heading = document.createElement('div');
  heading.className = 'edit-heading';
  const identity = document.createElement('span');
  identity.textContent = state.editSelection.target;
  const authority = document.createElement('span');
  authority.className = 'edit-authority';
  authority.textContent = 'source + plan locked';
  heading.append(identity, authority);

  const form = document.createElement('form');
  form.className = 'edit-form';
  form.setAttribute('aria-label', `Edit ${state.editSelection.target}`);
  const operationField = studioSelect('Handle', operations, operation);
  operationField.select.addEventListener('change', () => {
    state.editDraft = {operation: operationField.select.value, property: PaperStudioEditModel.properties(state.editSelection, operationField.select.value)[0]};
    renderEditControls();
  });
  const propertyField = studioSelect('Property', availableProperties, property);
  propertyField.select.addEventListener('change', () => {
    state.editDraft.property = propertyField.select.value;
    renderEditControls();
  });
  const valueSpec = PaperStudioEditModel.valueSpec(operation, property);
  if (operation === 'flow') {
    valueSpec.kind = 'choice';
    valueSpec.label = 'Destination';
    valueSpec.choices = PaperStudioEditModel.flowDestinations(state.editSelection).map((node) => node.id);
  }
  const valueField = studioValueField(valueSpec);
  const submit = document.createElement('button');
  submit.type = 'submit';
  submit.className = 'edit-commit';
  submit.textContent = state.committing ? 'Committing…' : 'Apply exact patch';
  submit.disabled = visualMutationsLocked();
  form.append(operationField.label, propertyField.label, valueField.label, submit);
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (visualMutationsLocked()) return;
    let payload;
    try {
      payload = PaperStudioEditModel.buildPayload(state.workspace, state.editSelection, operation, property, valueField.input.value);
    } catch (error) {
      state.editFeedback = {tone: 'error', text: error.message};
      renderEditControls();
      return;
    }
    state.committing = true;
    state.editFeedback = {tone: 'working', text: 'Committing against exact revisions…'};
    app.classList.add('is-committing');
    renderEditControls();
    try {
      const result = await api('/api/edit', {
        method: 'POST', headers: {'content-type': 'application/json'}, body: JSON.stringify(payload),
      });
      state.editFeedback = {tone: 'success', text: `Committed · ${result.patch_count} minimal patch`};
      await refresh();
    } catch (error) {
      if (error.status === 409) {
        state.editFeedback = {tone: 'stale', text: 'Stale selection · refreshed without applying'};
        await refresh();
      } else {
        state.editFeedback = {tone: 'error', text: error.message};
      }
    } finally {
      state.committing = false;
      app.classList.remove('is-committing');
      renderEditControls();
    }
  });
  target.append(heading, form);
  if (state.editFeedback) {
    const feedback = document.createElement('div');
    feedback.className = `edit-feedback is-${state.editFeedback.tone}`;
    feedback.textContent = state.editFeedback.text;
    target.append(feedback);
  }
}

function studioSelect(labelText, options, selected) {
  const label = document.createElement('label');
  label.className = 'edit-field';
  const caption = document.createElement('span');
  caption.textContent = labelText;
  const select = document.createElement('select');
  select.disabled = visualMutationsLocked();
  for (const value of options) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = value.replaceAll('-', ' ');
    option.selected = value === selected;
    select.append(option);
  }
  label.append(caption, select);
  return {label, select};
}

function studioValueField(spec) {
  const label = document.createElement('label');
  label.className = 'edit-field edit-value';
  const caption = document.createElement('span');
  caption.textContent = spec.label;
  let input;
  if (spec.kind === 'choice' || spec.kind === 'boolean') {
    input = document.createElement('select');
    for (const value of spec.choices) {
      const option = document.createElement('option');
      option.value = option.textContent = value;
      input.append(option);
    }
  } else {
    input = document.createElement('input');
    input.type = spec.kind === 'color' ? 'color' : spec.kind === 'text' ? 'text' : 'number';
    input.value = spec.kind === 'color' ? '#315ee8' : spec.kind === 'integer' ? '1' : spec.kind === 'text' ? '' : String(spec.min ?? 0);
    if (spec.min !== undefined) input.min = String(spec.min);
    if (spec.max !== undefined) input.max = String(spec.max);
    if (spec.step !== undefined) input.step = String(spec.step);
  }
  input.disabled = visualMutationsLocked();
  label.append(caption, input);
  return {label, input};
}

function flatten(value, prefix = '', result = []) {
  if (value === null || typeof value !== 'object') {
    result.push([prefix || 'value', value]);
    return result;
  }
  if (Array.isArray(value)) {
    value.forEach((item, index) => flatten(item, `${prefix}[${index}]`, result));
  } else {
    Object.entries(value).forEach(([key, entry]) => flatten(entry, prefix ? `${prefix}.${key}` : key, result));
  }
  return result;
}

function focusSourceLine(line) {
  const source = $('#source');
  const lineHeight = parseFloat(getComputedStyle(source).lineHeight) || 19.8;
  if (!['source', 'split'].includes(app.dataset.mode)) setMode('split');
  const row = source.querySelector(`.source-line[data-line="${Number(line)}"]`);
  source.querySelectorAll('.source-line.is-focused').forEach(item => item.classList.remove('is-focused'));
  document.querySelectorAll('.issue.is-selected').forEach(item => item.classList.remove('is-selected'));
  if (row) {
    row.classList.add('is-focused');
    source.scrollTop = Math.max(0, row.offsetTop - lineHeight * 3);
  } else {
    source.scrollTop = Math.max(0, (line - 4) * lineHeight);
  }
  const issue = document.querySelector(`.issue[data-line="${Number(line)}"]`);
  if (issue) {
    issue.classList.add('is-selected');
    issue.scrollIntoView({block: 'nearest'});
  }
}

function setMode(mode) {
  app.dataset.mode = mode;
  document.querySelectorAll('.mode').forEach((button) => {
    const active = button.dataset.mode === mode;
    button.classList.toggle('is-active', active);
    button.setAttribute('aria-pressed', String(active));
  });
  if (mode === 'review') app.classList.add('show-geometry');
  if (mode === 'accessibility') {
    document.querySelector('.overlay-disclosure').open = true;
    for (const overlay of ['reading', 'roles']) state.overlays.add(overlay);
    document.querySelectorAll('.inspection-toggle').forEach((button) => button.setAttribute('aria-pressed', String(state.overlays.has(button.dataset.overlay))));
    renderInspectionOverlays();
    renderPageInspectionEvidence();
    loadPDFTags();
  }
}

function renderDPIForZoom() {
  return Math.max(72, Math.min(600, Math.round(144 * Math.max(1, state.zoom))));
}

function applyPageStageWidth(rendered = state.activePageMeta) {
  if (!rendered) return;
  const naturalWidth = rendered.pixelWidth * 72 / rendered.manifest.profile.dpi;
  $('#page-stage').style.width = `${Math.max(1, naturalWidth * state.zoom)}px`;
}

function setZoom(next) {
  if (!Number.isFinite(next)) {
    $('#zoom-value').value = String(Math.round(state.zoom * 100));
    return;
  }
  state.zoom = Math.max(.25, Math.min(4, Math.round(next * 100) / 100));
  $('#zoom-value').value = String(Math.round(state.zoom * 100));
  applyPageStageWidth();
  const serial = ++state.zoomRender;
  const page = state.page;
  const revision = state.revision;
  const dpi = renderDPIForZoom();
  loadWASMPage(page, dpi).then(rendered => {
    if (serial !== state.zoomRender || page !== state.page || revision !== state.revision) return;
    paintWASMPage(rendered);
    renderInspectionOverlays();
    renderSelectionRects();
  }).catch(error => { if (error.status === 409) refresh(); else showFailure(error); });
}

function renderStatus() {
  const workspace = state.workspace;
  $('#page-label').textContent = workspace?.pages ? `Page ${state.page} of ${workspace.pages}` : 'No page';
  $('#cursor-state').textContent = workspace?.pages ? `Page ${state.page} · No selection` : 'No planned page';
  renderVerificationState();
}

function renderVerificationState() {
  const badge = $('#verification-state');
  if (!badge) return;
  let label = 'Plan preview';
  let className = 'is-preview';
  if (!state.workspace?.pages) {
    label = 'Unavailable';
    className = 'is-stale';
  } else if (app.classList.contains('is-stale') || state.verificationStale) {
    label = 'Verification stale';
    className = 'is-stale';
  } else if (state.pdfTagsRevision === state.revision && state.pdfTags?.passed) {
    label = 'PDF verified';
    className = 'is-verified';
  }
  badge.textContent = label;
  badge.className = `verification-state ${className}`;
  badge.title = label === 'PDF verified'
    ? 'The exact current plan was serialized and independently verified as a final PDF.'
    : label === 'Plan preview'
      ? 'The canvas is an exact plan preview; the final PDF has not been independently verified.'
      : 'The final-PDF verification no longer matches the current plan revision.';
}

function showFailure(error) {
  $('#connection-state').textContent = `Workspace error · ${error.message}`;
  app.classList.add('has-no-plan');
}

function clearObjectURLs() {
  for (const url of state.objectURLs) URL.revokeObjectURL(url);
  state.objectURLs.clear();
  for (const value of state.pageMeta.values()) value.bitmap?.close?.();
}

document.querySelectorAll('.mode').forEach((button) => button.addEventListener('click', () => setMode(button.dataset.mode)));
$('#toggle-overlay').addEventListener('click', (event) => {
  const enabled = app.classList.toggle('show-geometry');
  event.currentTarget.setAttribute('aria-pressed', String(enabled));
});
document.querySelectorAll('.inspection-toggle').forEach((button) => button.addEventListener('click', () => {
  const overlay = button.dataset.overlay;
  if (state.overlays.has(overlay)) state.overlays.delete(overlay); else state.overlays.add(overlay);
  button.setAttribute('aria-pressed', String(state.overlays.has(overlay)));
  renderInspectionOverlays();
  renderPageInspectionEvidence();
}));
overlapPicker.addEventListener('keydown', (event) => {
  const options = [...overlapPicker.querySelectorAll('[role="option"]')];
  const current = options.indexOf(document.activeElement);
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault();
    const delta = event.key === 'ArrowDown' ? 1 : -1;
    options[(current + delta + options.length) % options.length]?.focus();
  }
});
$('#refresh').addEventListener('click', () => refresh());
$('#zoom-in').addEventListener('click', () => setZoom(state.zoom + .1));
$('#zoom-out').addEventListener('click', () => setZoom(state.zoom - .1));
$('#zoom-value').addEventListener('change', event => setZoom(Number(event.currentTarget.value) / 100));
$('#zoom-value').addEventListener('keydown', event => {
  if (event.key !== 'Enter') return;
  event.preventDefault();
  setZoom(Number(event.currentTarget.value) / 100);
  event.currentTarget.blur();
});
pageImage.addEventListener('click', hitPage);
window.addEventListener('keydown', (event) => {
  if (event.target.matches('pre')) return;
  if (event.key === 'ArrowLeft') showPage(state.page - 1);
  if (event.key === 'ArrowRight') showPage(state.page + 1);
  if (event.key === '+' || event.key === '=') setZoom(state.zoom + .1);
  if (event.key === '-') setZoom(state.zoom - .1);
  if (event.key === '[') app.classList.toggle('left-collapsed');
  if (event.key === ']') app.classList.toggle('right-collapsed');
  if (event.key === 'Escape') {
    state.selectionFragments = [];
    renderSelectionRects();
    $('#selection-pulse').classList.remove('is-visible');
    closeOverlapPicker();
    renderInspector({}, 'Nothing selected');
    selectEditableTarget('');
  }
});
window.addEventListener('beforeunload', clearObjectURLs);

setZoom(1);
refresh();
state.poll = window.setInterval(() => refresh({quiet: true}), 2000);
