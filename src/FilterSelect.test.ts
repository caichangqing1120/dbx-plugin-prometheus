import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { compileTemplate, parse } from '@vue/compiler-sfc';
import { describe, expect, it } from 'vitest';

describe('FilterSelect pointer interaction', () => {
  it('keeps the menu mounted while clicking every selectable option', () => {
    const filename = fileURLToPath(new URL('./FilterSelect.vue', import.meta.url));
    const source = readFileSync(filename, 'utf8');
    const { descriptor } = parse(source, { filename });
    const result = compileTemplate({
      source: descriptor.template?.content || '',
      filename,
      id: 'filter-select-test',
    });

    expect(result.errors).toEqual([]);
    expect(result.code.match(/onMousedown:[^\n]+_withModifiers\(\(\) => \{\}, \["prevent"\]\)/g)).toHaveLength(2);
  });
});
