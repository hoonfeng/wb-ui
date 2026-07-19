"""扫描 WebKit JSC 源文件，生成完整文件清单"""
import os, sys

ROOT = r'F:\syproject\ref\WebKit\Source\JavaScriptCore'
OUT = r'F:\syproject\ref\JSC_File_Inventory.md'

# WebKit JSC 的核心子目录（排除 jit/dfg/ftl/b3/llint/assembler 等 JIT 目录）
TARGET_DIRS = [
    'runtime',
    'parser',
    'bytecompiler',
    'bytecode',
    'interpreter',
    'heap',
    'builtins',
    'API',
    'yarr',
    'debugger',
    'inspector',
    'tools',
    'wasm',
]

SKIP_DIRS = {'jit', 'dfg', 'ftl', 'b3', 'llint', 'assembler', 'Shell', 'Scripts', 'bmalloc'}

# 当前 wb-ui/jsc 目录
WB_UI_JSC = r'F:\syproject\wb-ui\jsc'

def classify_file(relpath):
    """判断一个 .h 文件是否需要翻译"""
    parts = relpath.replace('\\', '/').split('/')
    
    # 跳过 JIT 相关目录
    for skip in SKIP_DIRS:
        if skip in parts:
            return 'skip-jit'
    
    # 跳过 Inlines.h（内容合入主文件）
    if 'Inlines.h' in relpath or 'Inline.h' in relpath:
        return 'inline'
    
    # 跳过 test/fuzz 相关
    if 'test' in relpath.lower() or 'fuzz' in relpath.lower():
        return 'skip-test'
    
    # JIT 专用文件标记
    fname = parts[-1]
    jit_keywords = ['JIT', 'DFG', 'FTL', 'LLInt', 'linkBuffer', 'machineContext',
                    'MacroAssembler', 'RegisterAtOffset', 'TaggedPtr', 'Sentinel']
    for kw in jit_keywords:
        if kw in fname:
            return 'skip-jit'
    
    return 'translate'

def find_matching_go(h_path):
    """检查是否已有对应的 .go 文件"""
    # 从 h_path 提取相对路径
    rel = os.path.relpath(h_path, ROOT).replace('\\', '/')
    parts = rel.split('/')
    
    # 根目录文件跳过
    if len(parts) < 2:
        return ('skip-root', 0, None)
    
    module = parts[0]
    filename = parts[-1]
    if filename.endswith('.h'):
        go_name = filename[:-2] + '.go'
    elif filename.endswith('.cpp'):
        go_name = filename[:-4] + '.go'
    else:
        go_name = filename + '.go'
    
    # 查找 wb-ui/jsc/{module}/{go_name}
    go_path = os.path.join(WB_UI_JSC, module, go_name)
    if os.path.exists(go_path):
        size = os.path.getsize(go_path)
        return ('exists', size, go_path)
    return ('missing', 0, None)

def scan():
    if not os.path.isdir(ROOT):
        print(f"ERROR: {ROOT} not found")
        sys.exit(1)
    
    all_h_files = []
    
    for dirpath, dirnames, filenames in os.walk(ROOT):
        # 跳过隐藏目录和 JIT 目录
        rel = os.path.relpath(dirpath, ROOT).replace('\\', '/')
        
        # 只扫描目标子目录
        top_dir = rel.split('/')[0] if rel != '.' else ''
        if top_dir not in TARGET_DIRS and rel != '.':
            continue
        
        # 跳过 JIT 子目录
        skip = False
        for s in SKIP_DIRS:
            if s in rel.split('/'):
                skip = True
                break
        if skip:
            continue
        
        for f in sorted(filenames):
            if f.endswith('.h'):
                full = os.path.join(dirpath, f)
                relpath = os.path.relpath(full, ROOT)
                cls = classify_file(relpath)
                if cls == 'skip-jit':
                    continue
                all_h_files.append((relpath, cls))
    
    # 按模块分组（过滤掉根目录文件）
    modules = {}
    translate_count = 0
    skip_count = 0
    for relpath, cls in all_h_files:
        parts = relpath.replace('\\', '/').split('/')
        if len(parts) < 2:
            continue
        module = parts[0]
        if module not in modules:
            modules[module] = []
        modules[module].append((relpath, cls))
        if cls == 'translate':
            translate_count += 1
        else:
            skip_count += 1
    
    # 生成清单
    total = len(all_h_files)
    
    lines = [
        '# JSC 引擎完整文件清单',
        '',
        f'扫描时间: {__import__("datetime").datetime.now().strftime("%Y-%m-%d %H:%M:%S")}',
        f'扫描目录: {ROOT}',
        f'总文件数: {total}',
        '',
        '---',
        '',
        '## 文件清单',
        '',
    ]
    
    for module in sorted(modules.keys()):
        files = modules[module]
        lines.append(f'### {module}/ ({len(files)} 个文件)')
        lines.append('')
        lines.append('| # | 文件 | 分类 | 翻译状态 | Go 文件 | 大小 |')
        lines.append('|---|------|------|----------|--------|------|')
        
        for i, (relpath, cls) in enumerate(sorted(files, key=lambda x: x[0])):
            full_path = os.path.join(ROOT, relpath)
            result = find_matching_go(full_path)
            if result is None:
                continue
            status, size, go_path = result
            fname = relpath.replace('\\', '/')
            
            if cls == 'inline' or cls == 'skip-jit' or cls == 'skip-test':
                status_str = '⛔ 无需翻译'
            else:
                # 全部标记为未翻译——现有实现有错误，需要重译
                status_str = '⬜ 未翻译'
            
            size_str = f'{size} bytes' if size > 0 else '-'
            go_name = os.path.basename(go_path) if go_path else '-'
            
            lines.append(f'| {i+1} | `{fname}` | {cls} | {status_str} | `{go_name}` | {size_str} |')
        
        lines.append('')
    
    # 底部汇总
    lines.extend([
        '---',
        '',
        '## 汇总',
        '',
        f'| 指标 | 数值 |',
        f'|------|------|',
        f'| 总 .h 文件 | {total} |',
        f'| 需翻译 | {translate_count} |',
        f'| ⛔ 跳过(JIT/Inline/Test) | {skip_count} |',
        '',
        '---',
        '',
        '## 翻译进度追踪',
        '',
        '格式: `- [ ] module/filename` (未翻译) → `- [x] module/filename` (已完成)',
        '',
    ])
    
    # 生成任务列表
    task_lines = []
    for module in sorted(modules.keys()):
        files = modules[module]
        task_lines.append(f'### {module}/')
        task_lines.append('')
        for relpath, cls in sorted(files, key=lambda x: x[0]):
            if cls == 'translate':
                fname = relpath.replace('\\', '/')
                task_lines.append(f'- [ ] `{fname}`')
        task_lines.append('')
    
    lines.extend(task_lines)
    
    with open(OUT, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    with open(OUT, 'w', encoding='utf-8') as f:
        f.write('\n'.join(lines))
    
    print(f'清单已生成: {OUT}')
    print(f'总 .h 文件: {total}')
    print(f'需翻译: {translate_count}')
    print(f'跳过: {skip_count}')

if __name__ == '__main__':
    scan()
