#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
JSC C++ 头文件 -> Go 翻译管线工具（进度管理版）

用法:
  python translate_jsc.py done <相对路径>  -- 标记文件已完成
  python translate_jsc.py next             -- 显示下一个待翻译文件
  python translate_jsc.py list             -- 显示各模块完成进度

路径映射:
  源文件: ref/WebKit/Source/JavaScriptCore/{模块}/{文件名}.h
  输出:   wb-ui/jsc/{模块小写}/{蛇形文件名}.go

模块名映射:
  API -> api, builtins -> builtins, bytecode -> bytecode, bytecompiler -> bytecompiler,
  debugger -> debugger, heap -> heap, inspector -> inspector, interpreter -> interpreter,
  parser -> parser, runtime -> runtime, tools -> tools, wasm -> wasm, yarr -> yarr

注意：本工具只管理翻译进度（标记完成/查看进度），不自动生成骨架。
      实际翻译工作由 agent 按 jsc-translator 技能规范手动完成。
"""

import sys
import os
import re

# ===== 路径配置 =====
INVENTORY_PATH = r'F:\syproject\ref\JSC_File_Inventory.md'
WEBKIT_ROOT = r'F:\syproject\ref\WebKit\Source\JavaScriptCore'
WB_UI_ROOT = r'F:\syproject\wb-ui'
JSC_OUTPUT_ROOT = os.path.join(WB_UI_ROOT, 'jsc')

# ===== 模块名映射 =====
MODULE_MAP = {
    'API': 'api',
    'builtins': 'builtins',
    'bytecode': 'bytecode',
    'bytecompiler': 'bytecompiler',
    'debugger': 'debugger',
    'heap': 'heap',
    'inspector': 'inspector',
    'interpreter': 'interpreter',
    'parser': 'parser',
    'runtime': 'runtime',
    'tools': 'tools',
    'wasm': 'wasm',
    'yarr': 'yarr',
}


def load_inventory():
    """解析清单文件，返回文件条目列表"""
    with open(INVENTORY_PATH, 'r', encoding='utf-8') as f:
        content = f.read()

    files = []
    in_task_section = False

    for line in content.split('\n'):
        stripped = line.strip()

        if stripped == '## 翻译进度追踪':
            in_task_section = True
            continue

        if in_task_section:
            if stripped.startswith('## ') and stripped != '## 翻译进度追踪':
                in_task_section = False
                continue

            m = re.match(r'- \[( |x)\] `(.+?)`', stripped)
            if m:
                done = m.group(1) == 'x'
                path = m.group(2)

                # 检查源文件是否存在
                source_path = os.path.join(WEBKIT_ROOT, path.replace('/', '\\'))
                source_exists = os.path.exists(source_path)

                # 计算输出路径
                parts = path.split('/')
                module = parts[0]
                filename = parts[-1]
                module_lower = MODULE_MAP.get(module, module.lower())
                output_module_dir = os.path.join(JSC_OUTPUT_ROOT, module_lower)
                basename = filename[:-2]
                snake_name = camel_to_snake(basename)
                go_name = snake_name + '.go'
                output_path = os.path.join(output_module_dir, go_name)
                output_exists = os.path.exists(output_path)
                output_size = os.path.getsize(output_path) if output_exists else 0

                files.append({
                    'module': module_lower,
                    'original_module': module,
                    'path': path,
                    'done': done,
                    'source_exists': source_exists,
                    'source_path': source_path,
                    'output_path': output_path,
                    'output_exists': output_exists,
                    'output_size': output_size,
                    'original_line': stripped,
                })

    return files


def save_inventory(files):
    """将文件状态写回清单的翻译进度追踪章节"""
    with open(INVENTORY_PATH, 'r', encoding='utf-8') as f:
        content = f.read()

    lines = content.split('\n')
    new_lines = []
    file_idx = 0
    in_task_section = False

    for line in lines:
        stripped = line.strip()

        if stripped == '## 翻译进度追踪':
            in_task_section = True
            new_lines.append(line)
            continue

        if in_task_section:
            m = re.match(r'- \[( |x)\] `(.+?)`', stripped)
            if m:
                if file_idx < len(files):
                    status = 'x' if files[file_idx]['done'] else ' '
                    new_lines.append('- [{0}] `{1}`'.format(status, files[file_idx]["path"]))
                    file_idx += 1
                else:
                    new_lines.append(line)
                continue
            elif stripped.startswith('### ') or stripped.startswith('## '):
                in_task_section = False
                new_lines.append(line)
                continue
            else:
                new_lines.append(line)
        else:
            new_lines.append(line)

    with open(INVENTORY_PATH, 'w', encoding='utf-8') as f:
        f.write('\n'.join(new_lines))


def camel_to_snake(name):
    """CamelCase -> snake_case 转换"""
    name = re.sub(r'([A-Z]+)([A-Z][a-z])', r'\1_\2', name)
    name = re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', name)
    return name.lower()


def get_pipeline_files(files):
    """按执行管线优先级排序的文件列表"""
    pipeline_paths = [
        # Phase 1: Parser
        'parser/Lexer.h', 'parser/Nodes.h', 'parser/NodeConstructors.h',
        'parser/ASTBuilder.h', 'parser/Parser.h', 'parser/SyntaxChecker.h',
        'parser/ParserArena.h', 'parser/ParserFunctionInfo.h', 'parser/ParserModes.h',
        'parser/ParserTokens.h', 'parser/ParserError.h', 'parser/ResultType.h',
        'parser/SourceCode.h', 'parser/SourceProvider.h', 'parser/SourceProviderCache.h',
        'parser/SourceProviderCacheItem.h', 'parser/UnlinkedSourceCode.h',
        'parser/VariableEnvironment.h', 'parser/ModuleAnalyzer.h', 'parser/ModuleScopeData.h',

        # Phase 2: BytecodeGenerator
        'bytecompiler/BytecodeGenerator.h', 'bytecompiler/BytecodeGeneratorBase.h',
        'bytecompiler/RegisterID.h', 'bytecompiler/Label.h', 'bytecompiler/LabelScope.h',
        'bytecompiler/ProfileTypeBytecodeFlag.h', 'bytecompiler/StaticPropertyAnalysis.h',
        'bytecompiler/StaticPropertyAnalyzer.h',

        # Phase 3: CodeBlock
        'bytecode/CodeBlock.h', 'bytecode/UnlinkedCodeBlock.h', 'bytecode/CodeBlockHash.h',
        'bytecode/Instruction.h', 'bytecode/InstructionStream.h',

        # Phase 4: Interpreter
        'interpreter/Interpreter.h', 'interpreter/CallFrame.h', 'interpreter/Register.h',
        'interpreter/CLoopStack.h', 'interpreter/StackVisitor.h',
    ]

    pipeline_dict = {p: i for i, p in enumerate(pipeline_paths)}
    pipeline_files = [f for f in files if f['path'] in pipeline_dict]
    pipeline_files.sort(key=lambda f: pipeline_dict[f['path']])

    other_files = [f for f in files if f['path'] not in pipeline_dict]
    other_files.sort(key=lambda f: f['path'])

    return pipeline_files, other_files


def cmd_done(path):
    """标记文件已完成"""
    files = load_inventory()
    found = False

    normalized_path = path.replace('\\', '/')

    for f in files:
        if f['path'] == normalized_path:
            f['done'] = True
            found = True
            break

    if not found:
        print("错误: 未找到路径 '{0}'".format(normalized_path))
        print("请使用类似 'API/APICallbackFunction.h' 的格式")
        return False

    save_inventory(files)
    print("已完成: {0}".format(normalized_path))
    return True


def cmd_next():
    """显示下一个待翻译文件的信息（按管线优先级）"""
    files = load_inventory()

    pipeline_files, other_files = get_pipeline_files(files)
    all_sorted = pipeline_files + other_files

    # 找到第一个未完成的文件
    target = None
    for f in all_sorted:
        if not f['done'] and f['source_exists']:
            target = f
            break

    if not target:
        print("所有文件均已翻译完成！")
        return

    print("下一个待翻译文件:")
    print("  清单路径:  {0}".format(target['path']))
    print("  源文件:    {0}".format(target['source_path']))
    print("  源文件存在: {0}".format('YES' if target['source_exists'] else 'NO'))
    print("  输出:      {0}".format(target['output_path']))
    print("  模块:      {0}".format(target['module']))

    if target['output_exists']:
        print("  状态:      已有 Go 文件 ({0} bytes)，尚未标记完成".format(target['output_size']))
    else:
        print("  状态:      未翻译")

    # 显示管线中接下来的几个未完成文件
    pipeline_pending = [f for f in pipeline_files if not f['done'] and f['source_exists']]
    if pipeline_pending:
        print("\n  执行管线中待翻译 ({0} 个):".format(len(pipeline_pending)))
        for i, f in enumerate(pipeline_pending[:5]):
            status = "GO" if f['output_exists'] else "  "
            print("    {0}. [{1}] {2}".format(i+1, status, f['path']))
        if len(pipeline_pending) > 5:
            print("    ... 还有 {0} 个".format(len(pipeline_pending) - 5))


def cmd_list():
    """显示各模块翻译进度"""
    files = load_inventory()
    total = len([f for f in files if f['source_exists']])
    done = sum(1 for f in files if f['done'])
    has_go = sum(1 for f in files if f['output_exists'])

    if total == 0:
        print("\n清单中没有可翻译的文件。")
        return

    print("\n=== JSC 翻译进度: {0}/{1} ({2}%) ===\n".format(done, total, done * 100 // total))

    # 按模块分组
    modules = {}
    for f in files:
        m = f['module']
        if m not in modules:
            modules[m] = {'total': 0, 'done': 0, 'has_go': 0, 'size': 0}
        if f['source_exists']:
            modules[m]['total'] += 1
            if f['done']:
                modules[m]['done'] += 1
            if f['output_exists']:
                modules[m]['has_go'] += 1
                modules[m]['size'] += f['output_size']

    # 打印模块进度
    for m in sorted(modules.keys()):
        d = modules[m]
        if d['total'] == 0:
            pct = 0
        else:
            pct = d['done'] * 100 // d['total']
        bar_len = 20
        filled = pct * bar_len // 100
        bar = '#' * filled + '-' * (bar_len - filled)

        if d['size'] > 1024:
            size_str = "({0}KB)".format(d['size'] // 1024)
        else:
            size_str = "({0}B)".format(d['size'])
        go_str = " go:{0}".format(d['has_go']) if d['has_go'] else ""
        label = "{0:16s}".format(m)
        print("  {0} [{1}] {2:4d}/{3:<4d} ({4:2d}%){5} {6}".format(label, bar, d['done'], d['total'], pct, go_str, size_str))

    print("\n  总计: {0}/{1} ({2}%)  Go文件: {3}".format(done, total, done * 100 // total, has_go))

    # 执行管线进度
    pipeline_files, _ = get_pipeline_files(files)
    pipeline_done = sum(1 for f in pipeline_files if f['done'])
    pipeline_total = len(pipeline_files)
    if pipeline_total > 0:
        p_pct = pipeline_done * 100 // pipeline_total
        p_bar = '#' * (p_pct // 5) + '-' * (20 - p_pct // 5)
        print("\n  执行管线: [{0}] {1}/{2} ({3}%)".format(p_bar, pipeline_done, pipeline_total, p_pct))
        for f in pipeline_files:
            status = 'x' if f['done'] else ' '
            has = 'GO' if f['output_exists'] else '  '
            size = '({0}b)'.format(f['output_size']) if f['output_exists'] else ''
            print("    [{0}] {1} {2} {3}".format(status, has, f['path'], size))

    # 统计 Go 代码量
    total_size = sum(f['output_size'] for f in files)
    print("\n  总代码量: {0}KB ({1} bytes)".format(total_size // 1024, total_size))


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(1)

    cmd = sys.argv[1]

    if cmd == 'done':
        if len(sys.argv) < 3:
            print("用法: python translate_jsc.py done <相对路径>")
            print("示例: python translate_jsc.py done 'API/APICallbackFunction.h'")
            sys.exit(1)
        success = cmd_done(sys.argv[2])
        sys.exit(0 if success else 1)
    elif cmd == 'next':
        cmd_next()
    elif cmd == 'list':
        cmd_list()
    else:
        print("未知命令: {0}".format(cmd))
        print(__doc__)
        sys.exit(1)


if __name__ == '__main__':
    main()
