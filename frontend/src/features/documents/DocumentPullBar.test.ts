// @vitest-environment happy-dom
import { afterEach, expect, it } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NButton, NSelect } from 'naive-ui'
import DocumentPullBar from './DocumentPullBar.vue'
import DocumentPullSettings from './DocumentPullSettings.vue'
import { editableConditions, conditionsPending } from './pullConditions'
import type { Document } from '../../shared/types/types'
const doc: Document = {key:'a'.repeat(24), version:1, title:'个人任务', owner:'',source_url:'https://docs.qq.com/sheet/test',source_path:'',cached:true,updated_at:'2026-10-18',warnings:[],include_completed:false,sheets:[{name:'任务表',headers:['事项','模块','状态','备注'],header_row:1,scanned:0,rows:[]}]}
let wrapper: ReturnType<typeof mount> | undefined
function setup(current = doc, pendingEdits = false) {
 const editor=mount(DocumentPullSettings,{props:{document:structuredClone(current),modelValue:editableConditions(current),disabled:false,pendingEdits,
   'onUpdate:modelValue': value => { void wrapper!.setProps({modelValue:value}) },
 },attachTo:document.body})
 wrapper=editor
 return editor
}
afterEach(()=>{wrapper?.unmount();wrapper=undefined;document.body.innerHTML=''})

it('edits any header as a draft and removes the last condition without a forced owner',async()=>{
 const editor=setup()
 await editor.findComponent(NButton).trigger('click');await flushPromises()
 const field=editor.findAllComponents(NSelect)[0]
 expect((field.props('options') as {header:string}[]).map(o=>o.header)).toEqual(doc.sheets[0].headers)
 field.vm.$emit('update:value','任务表:2');await flushPromises()
 await editor.find('input[aria-label="条件 1 内容"]').setValue('后端');await flushPromises()
 expect(editor.props('modelValue')).toEqual({owner:'',include_completed:true,filters:[{sheet:'任务表',column:2,header:'模块',operator:'equals',value:'后端'}]})
 await editor.find('[aria-label="删除条件 1"]').trigger('click');await flushPromises()
 expect(editor.props('modelValue')).toEqual({owner:'',include_completed:true,filters:[]})
})
it('migrates an old owner to a removable member condition',async()=>{
 const legacy={...doc,include_completed:true,owner:'王小明',sheets:[{...doc.sheets[0],headers:['事项','负责人员','状态','备注']}]}
 const editor=setup(legacy)
 expect(editor.find<HTMLInputElement>('input').element.value).toBe('王小明')
 expect(editor.findAllComponents(NSelect)[1].props('value')).toBe('member')
 await editor.find('[aria-label="删除条件 1"]').trigger('click');await flushPromises()
 expect(editor.props('modelValue')).toEqual({owner:'',include_completed:true,filters:[]})
})
it('blocks condition editing with pending local changes',()=>{
 const editor=setup({...doc,owner:'王小明'},true)
 expect(editor.findAllComponents(NButton).every(button=>button.props('disabled'))).toBe(true)
 expect(editor.text()).toContain('请先回写或撤销')
})
it('shows only a text summary and distinguishes saved conditions from the applied snapshot',()=>{
 wrapper=mount(DocumentPullBar,{props:{document:{...doc,owner:'王小明'}}})
 expect(wrapper.text()).toContain('负责人 包含成员 王小明')
 expect(wrapper.find('button, input, [role="button"]').exists()).toBe(false)
 const desired={owner:'',include_completed:true,filters:[]}
 expect(conditionsPending({...doc,pull_conditions:desired})).toBe(true)
 expect(conditionsPending({...doc,pull_conditions:desired,applied_pull_conditions:desired})).toBe(false)
 expect(conditionsPending({...doc,setup_pending:true})).toBe(true)
})
