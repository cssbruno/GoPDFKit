const assert = require('node:assert/strict');
const model = require('../web/authoring-model.js');
const workspace = {revision:'plan-1', plan_hash:'plan-1', source_revision:'source-1', scenario:''};
const payload = {format_version:1, revision:'plan-1', plan_hash:'plan-1', source_revision:'source-1', document_target:'@doc',
  template_targets:[{id:'@body',kind:'body'}], binding_targets:[{id:'@copy',kind:'paragraph'}],
  schemas:[{name:'@invoice',fields:[{path:'@invoice.total',kind:'number',required:true}]}], scenarios:[], stress_presets:['empty','typical','stress'], components:['@card']};
const metadata = model.normalize(payload, workspace);
assert.deepEqual(model.buildPayload(workspace, metadata, {operation:'template',target:'@body',template:'section',id:'@summary'}),
  {source_revision:'source-1',plan_revision:'plan-1',scenario:'',operation:'template',property:'',target:'@body',template:'section',id:'@summary'});
const bootstrapMetadata = model.normalize({...payload, template_targets:[{id:'@doc',kind:'document'}]}, workspace);
assert.equal(model.buildPayload(workspace, bootstrapMetadata, {operation:'template',target:'@doc',template:'page',id:'@sheet'}).template, 'page');
assert.throws(() => model.buildPayload(workspace, bootstrapMetadata, {operation:'template',target:'@doc',template:'section',id:'@bad'}), /compatible/);
assert.equal(model.buildPayload(workspace, metadata, {operation:'binding',target:'@copy',path:'@invoice.total',required:true}).path, '@invoice.total');
assert.equal(model.buildPayload(workspace, metadata, {operation:'scenario-create',target:'@doc',schema:'@invoice',preset:'stress',id:'@stress'}).preset, 'stress');
assert.equal(model.buildPayload(workspace, metadata, {operation:'template',target:'@body',template:'component',component:'@card',id:'@card-1'}).component, '@card');
assert.equal(model.buildPayload(workspace, metadata, {operation:'template',target:'@body',template:'heading',id:'@heading'}).template, 'heading');
assert.throws(() => model.normalize({...payload, revision:'old'}, workspace), /Stale/);
assert.throws(() => model.normalize({...payload, source_revision:'old'}, workspace), /Stale/);
assert.throws(() => model.buildPayload(workspace, metadata, {operation:'binding',target:'@copy',path:'invented'}), /compiler-provided/);
console.log('authoring model tests passed');
