// @vitest-environment jsdom
import { expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import MarkdownDocument from './MarkdownDocument.vue'
it('renders Markdown while removing executable HTML from task and AI text', () => {
 const wrapper = mount(MarkdownDocument, { props: { source: '# 执行记录\n\n- **完成验证**\n\n<script>alert(1)</script><img src=x onerror="alert(1)"><a href="javascript:alert(1)">链接</a>' } })
 expect(wrapper.find('h1').text()).toBe('执行记录')
 expect(wrapper.find('strong').text()).toBe('完成验证')
 expect(wrapper.find('script').exists()).toBe(false)
 expect(wrapper.find('img').exists()).toBe(false)
 expect(wrapper.html()).not.toContain('javascript:')
 wrapper.unmount()
})
