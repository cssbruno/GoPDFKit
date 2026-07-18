(function (root, factory) {
  const model = factory();
  if (typeof module === 'object' && module.exports) module.exports = model;
  else root.PaperStudioResourceModel = model;
})(typeof globalThis !== 'undefined' ? globalThis : this, function () {
  'use strict';
  function normalize(payload, revision, sourceRevision, planHash) {
    if (!payload || payload.format_version !== 1 || payload.revision !== revision || payload.source_revision !== sourceRevision || !planHash || payload.plan_hash !== planHash) throw new Error('Resource inventory belongs to a stale or unsupported plan');
    return (payload.items || []).map((item) => ({name:String(item.name),kind:String(item.kind||'image'),mediaType:String(item.media_type),digest:String(item.digest),bytes:Number(item.bytes),width:Number(item.width_px||0),height:Number(item.height_px||0),family:String(item.family||''),weight:Number(item.weight||0),style:String(item.style||''),license:String(item.license||''),fallback:[...(item.fallback||[])],replaces:String(item.replaces||''),defaultFocusX:item.default_focus_x??null,defaultFocusY:item.default_focus_y??null,usages:(item.usages||[]).map((usage)=>({...usage}))}));
  }
  function usageLabel(item) { const count=item.usages.length; return item.kind==='font'?`${item.family} · ${item.weight} ${item.style} · ${item.bytes} bytes`:`${item.width}×${item.height} · ${item.bytes} bytes · ${count} ${count===1?'use':'uses'}`; }
  function replacementPayload(workspace, items, target, replacement) {
    if(!workspace?.source_revision||!workspace?.revision)throw new Error('Exact resource revisions are unavailable');
    const next=items.find(item=>item.kind==='image'&&item.name===replacement&&item.replaces);const previous=next&&items.find(item=>item.name===next.replaces);
    if(!next||!previous?.usages.some(usage=>usage.node===target))throw new Error('Replacement is not declared for this exact usage');
    return {source_revision:workspace.source_revision,plan_revision:workspace.revision,scenario:workspace.scenario||'',operation:'image',target,property:'source',text:`asset:${replacement}`};
  }
  function catalogAddPayload(workspace, fields) {
    if(!workspace?.source_revision||!workspace?.revision)throw new Error('Exact resource revisions are unavailable');
    if(!fields?.name||!fields?.path||!fields?.mediaType)throw new Error('Name, project-relative path, and media type are required');
    const payload={source_revision:workspace.source_revision,plan_revision:workspace.revision,scenario:workspace.scenario||'',operation:'add',name:String(fields.name),path:String(fields.path),media_type:String(fields.mediaType)};
    for(const [key,value] of [['family',fields.family],['style',fields.style],['license',fields.license],['replaces',fields.replaces]])if(value)payload[key]=String(value);
    if(fields.weight!==undefined&&fields.weight!=='')payload.weight=Number(fields.weight);
    if(Array.isArray(fields.fallback)&&fields.fallback.length)payload.fallback=fields.fallback.map((value)=>String(value)).filter(Boolean);
    if(fields.focusX!==undefined&&fields.focusX!=='')payload.focus_x=Number(fields.focusX);
    if(fields.focusY!==undefined&&fields.focusY!=='')payload.focus_y=Number(fields.focusY);
    return payload;
  }
  function catalogRemovePayload(workspace, name) {
    if(!workspace?.source_revision||!workspace?.revision)throw new Error('Exact resource revisions are unavailable');
    if(!name)throw new Error('Resource name is required');
    return {source_revision:workspace.source_revision,plan_revision:workspace.revision,scenario:workspace.scenario||'',operation:'remove',name:String(name)};
  }
  return Object.freeze({normalize, usageLabel, replacementPayload, catalogAddPayload, catalogRemovePayload});
});
