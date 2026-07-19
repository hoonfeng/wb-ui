"""JSC 翻译工作流管理器 v2 — 聚焦执行管线
用法:
  python jsc_workflow.py list              — 显示进度
  python jsc_workflow.py next              — 获取下一个待翻译文件
  python jsc_workflow.py done <file>       — 标记文件已完成
  python jsc_workflow.py module <name>     — 查看某模块文件
  python jsc_workflow.py reprioritize      — 重新优先级排序
"""
import sys, os, re, json

INVENTORY = r'F:\syproject\ref\JSC_File_Inventory.md'
WB_UI_JSC = r'F:\syproject\wb-ui\jsc'
WEBKIT_ROOT = r'F:\syproject\ref\WebKit\Source\JavaScriptCore'

# 执行管线文件 — 这些是让引擎工作的关键
# 按严格依赖顺序排列
PIPELINE_PRIORITY = [
    # Phase 1: Parser (当前13%, 需+7000行)
    'parser/Lexer.h',
    'parser/Nodes.h',
    'parser/NodeConstructors.h',
    'parser/ASTBuilder.h',
    'parser/Parser.h',
    'parser/SyntaxChecker.h',
    'parser/ParserArena.h',
    'parser/ParserFunctionInfo.h',
    'parser/ParserModes.h',
    'parser/ParserTokens.h',
    'parser/ParserError.h',
    'parser/ResultType.h',
    'parser/SourceCode.h',
    'parser/SourceProvider.h',
    'parser/SourceProviderCache.h',
    'parser/SourceProviderCacheItem.h',
    'parser/UnlinkedSourceCode.h',
    'parser/VariableEnvironment.h',
    'parser/ModuleAnalyzer.h',
    'parser/ModuleScopeData.h',
    
    # Phase 2: BytecodeGenerator (当前8%, 需+7000行)
    'bytecompiler/BytecodeGenerator.h',
    'bytecompiler/BytecodeGeneratorBase.h',
    'bytecompiler/RegisterID.h',
    'bytecompiler/Label.h',
    'bytecompiler/LabelScope.h',
    'bytecompiler/ProfileTypeBytecodeFlag.h',
    'bytecompiler/StaticPropertyAnalysis.h',
    'bytecompiler/StaticPropertyAnalyzer.h',
    
    # Phase 3: CodeBlock (当前3%, 需+5000行)
    'bytecode/CodeBlock.h',
    'bytecode/UnlinkedCodeBlock.h',
    'bytecode/CodeBlockHash.h',
    'bytecode/Instruction.h',
    'bytecode/InstructionStream.h',
    
    # Phase 4: Interpreter (当前5%, 需+2000行)
    'interpreter/Interpreter.h',
    'interpreter/CallFrame.h',
    'interpreter/Register.h',
    'interpreter/CLoopStack.h',
    'interpreter/StackVisitor.h',
]


def load_inventory():
    """解析清单"""
    with open(INVENTORY, 'r', encoding='utf-8') as f:
        content = f.read()
    
    files = []
    in_task = False
    
    for line in content.split('\n'):
        if line.strip() == '## 翻译进度追踪':
            in_task = True
            continue
        if in_task:
            m = re.match(r'- \[( |x)\] `(.+?)`', line)
            if m:
                done = m.group(1) == 'x'
                path = m.group(2)
                # 判断是否存在 .go 文件
                parts = path.split('/')
                if len(parts) >= 2:
                    module = parts[0]
                    go_name = parts[-1].replace('.h', '.go')
                    go_path = os.path.join(WB_UI_JSC, module, go_name)
                    has_go = os.path.exists(go_path)
                    go_size = os.path.getsize(go_path) if has_go else 0
                else:
                    module = 'unknown'
                    has_go = False
                    go_size = 0
                
                files.append({
                    'module': module,
                    'path': path,
                    'done': done,
                    'has_go': has_go,
                    'go_size': go_size,
                    'original_line': line,
                })
    
    return files


def save_inventory(files):
    """写回清单"""
    with open(INVENTORY, 'r', encoding='utf-8') as f:
        content = f.read()
    
    lines = content.split('\n')
    new_lines = []
    task_idx = 0
    in_task = False
    
    for line in lines:
        if line.strip() == '## 翻译进度追踪':
            in_task = True
            new_lines.append(line)
            continue
        if in_task:
            m = re.match(r'- \[( |x)\] `(.+?)`', line)
            if m and task_idx < len(files):
                status = 'x' if files[task_idx]['done'] else ' '
                new_lines.append(f'- [{status}] `{files[task_idx]["path"]}`')
                task_idx += 1
                continue
            elif line.strip().startswith('### '):
                in_task = False
                new_lines.append(line)
            elif not line.strip():
                in_task = False
                new_lines.append(line)
            else:
                new_lines.append(line)
        else:
            new_lines.append(line)
    
    with open(INVENTORY, 'w', encoding='utf-8') as f:
        f.write('\n'.join(new_lines))


def get_next_file():
    """获取下一个需翻译的文件"""
    files = load_inventory()
    
    # 先将执行管线文件按优先级排序
    pipeline_files = []
    other_files = []
    
    for f in files:
        if not f['done'] and f['path'] in PIPELINE_PRIORITY:
            pipeline_files.append(f)
        elif not f['done']:
            other_files.append(f)
    
    pipeline_files.sort(key=lambda f: PIPELINE_PRIORITY.index(f['path']) 
                        if f['path'] in PIPELINE_PRIORITY else 999)
    
    all_sorted = pipeline_files + other_files
    
    for f in all_sorted:
        src = os.path.join(WEBKIT_ROOT, f['path'].replace('/', '\\'))
        if os.path.exists(src):
            return f
    
    return None


def mark_done(path):
    """标记文件为已完成"""
    files = load_inventory()
    found = False
    for f in files:
        if f['path'] == path:
            f['done'] = True
            found = True
            break
    if not found:
        print(f"ERROR: '{path}' not found")
        return False
    save_inventory(files)
    print(f"Marked done: {path}")
    return True


def auto_detect_completed():
    """自动检测已有 .go 文件的翻译项"""
    files = load_inventory()
    changed = 0
    for f in files:
        parts = f['path'].split('/')
        if len(parts) >= 2:
            module = parts[0]
            go_name = parts[-1].replace('.h', '.go')
            go_path = os.path.join(WB_UI_JSC, module, go_name)
            if os.path.exists(go_path) and os.path.getsize(go_path) > 100:
                if not f['done']:
                    f['done'] = True
                    changed += 1
    
    if changed:
        save_inventory(files)
        print(f"Auto-detected {changed} completed files")
    return changed


def show_progress():
    """显示进度"""
    files = load_inventory()
    total = len(files)
    done = sum(1 for f in files if f['done'])
    
    print(f"\n=== Progress: {done}/{total} ({done*100//max(total,1)}%) ===\n")
    
    # Pipeline progress
    pipeline_done = sum(1 for f in files if f['path'] in PIPELINE_PRIORITY and f['done'])
    pipeline_total = len(PIPELINE_PRIORITY)
    pct = pipeline_done * 100 // pipeline_total
    bar = '#' * (pct // 5) + '-' * (20 - pct // 5)
    print(f"  EXECUTION PIPELINE: [{bar}] {pipeline_done}/{pipeline_total} ({pct}%)")
    print()
    
    # Module stats
    modules = {}
    for f in files:
        m = f['module']
        if m not in modules:
            modules[m] = {'total': 0, 'done': 0, 'has_go': 0}
        modules[m]['total'] += 1
        if f['done']:
            modules[m]['done'] += 1
        if f['has_go']:
            modules[m]['has_go'] += 1
    
    for m in sorted(modules.keys()):
        d = modules[m]
        pct = d['done'] * 100 // d['total'] if d['total'] else 0
        bar = '#' * (pct // 5) + '-' * (20 - pct // 5)
        go_mark = f" (have {d['has_go']} go files)" if d['has_go'] else ""
        print(f"  {m:15s} [{bar:20s}] {d['done']:4d}/{d['total']:<4d} ({pct:2d}%){go_mark}")
    
    # Pipeline specific
    print("\n--- Execution Pipeline Files ---")
    for p in PIPELINE_PRIORITY:
        found = [f for f in files if f['path'] == p]
        if found:
            f = found[0]
            status = 'x' if f['done'] else ' '
            has = 'GO' if f['has_go'] else '  '
            size = f'({f["go_size"]}b)' if f['has_go'] else ''
            print(f"  [{status}] {has} {p} {size}")
    

def show_module(module_name):
    """显示某模块文件"""
    files = load_inventory()
    mod_files = [f for f in files if f['module'] == module_name]
    mod_files.sort(key=lambda f: f['path'])
    
    untranslated = [f for f in mod_files if not f['done']]
    translated = [f for f in mod_files if f['done']]
    
    print(f"\n=== {module_name}/ ({len(mod_files)} files) ===")
    print(f"  Done: {len(translated)}, Left: {len(untranslated)}\n")
    
    if untranslated:
        print("  --- Untranslated ---")
        for f in untranslated[:40]:
            has = 'GO' if f['has_go'] else '  '
            print(f"    [{has}] `{f['path']}`")


if __name__ == '__main__':
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)
    
    cmd = sys.argv[1]
    
    if cmd == 'list':
        show_progress()
    elif cmd == 'next':
        n = get_next_file()
        if n:
            print(n['path'])
        else:
            print("ALL DONE!")
    elif cmd == 'done':
        if len(sys.argv) < 3:
            print("Usage: python jsc_workflow.py done <path>")
            sys.exit(1)
        mark_done(sys.argv[2])
    elif cmd == 'module':
        if len(sys.argv) < 3:
            print("Usage: python jsc_workflow.py module <name>")
            sys.exit(1)
        show_module(sys.argv[2])
    elif cmd == 'scan':
        auto_detect_completed()
    else:
        print(f"Unknown: {cmd}")
