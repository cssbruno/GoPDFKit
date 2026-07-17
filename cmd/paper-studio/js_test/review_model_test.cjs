const assert = require('node:assert/strict');
const model = require('../web/review-model.js');

const root = {kind:'document', members:[
  {node:{kind:'component', id:'@card', members:[{node:{kind:'slot', id:'@content', members:[]}}]}},
  {node:{kind:'page', id:'@page', members:[{node:{kind:'body', id:'@body', members:[
    {node:{kind:'use', id:'@one', members:[{property:{name:'component', value:{string_value:'@card'}}}]}},
    {node:{kind:'paragraph', id:'@copy', members:[{property:{name:'bind', value:{string_value:'@invoice.total'}}},{property:{name:'style', value:{string_value:'@body'}}}]}},
    {node:{kind:'paragraph', id:'@copy-2', members:[{property:{name:'bind', value:{string_value:'@invoice.total'}}},{property:{name:'style', value:{string_value:'@body'}}}]}},
  ]}}]}}
]};

assert.deepEqual(model.acceptedPalette('body').map(item => item.kind), ['paragraph', 'heading', 'list', 'row', 'column', 'page-break', 'component']);
assert.deepEqual(model.dropTargets(root), [
  {target:'@body', kind:'body', accepts:['paragraph','heading','list','row','column','page-break','component']},
  {target:'@one', slot:'@content', kind:'slot', accepts:['paragraph','heading','list','row','column','page-break','component']},
]);
assert.equal(model.describe(root.members[1].node.members[0].node.members[1].node, root).mode, 'binding');
assert.equal(model.describe(root.members[1].node.members[0].node.members[0].node, root).mode, 'invocation');
assert.equal(model.blastRadius(root.members[0].node, root).scope, 'local');
assert.deepEqual(model.blastRadius(root.members[1].node.members[0].node.members[1].node, root), {scope:'shared', count:2, targets:['@copy','@copy-2']});
assert.equal(model.pageBreakPolicies().length, 3);
assert.equal(model.optimisticFeedback('box background').authoritative, false);
const workspace = {revision:'plan-1', source_revision:'source-1', plan_hash:'plan-1', scenario:'@preview'};
const normalized = model.normalizeReview({format_version:1, revision:'plan-1', source_revision:'source-1', plan_hash:'plan-1', annotations:[{transform:[1,0,0,1,0,0]}], comments:[]}, workspace);
assert.equal(normalized.annotations[0].transform.length, 6);
assert.equal(model.commentPayload(workspace, '@copy', 1, 'keep semantic target', 'reviewer').kind, 'comment');
assert.equal(model.annotationPayload(workspace, '@copy', 1, 'pin').transform.length, 6);
assert.throws(() => model.commentPayload(workspace, '@copy', 1, ''));
console.log('review model tests passed');
