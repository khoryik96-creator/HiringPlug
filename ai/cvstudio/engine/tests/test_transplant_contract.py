"""New handover integration checks using only fictional CV data."""
import copy
import io
import json
from pathlib import Path
import zipfile

from docx import Document
from lxml import etree

from portable_pipeline import postprocess_parse, prepare_export, render
from cvstudio_docx_extract import _extract_docx_text_preserve_tables, _extract_docx_textbox_lines

ROOT = Path(__file__).resolve().parents[2]
NS = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}


def example():
    return (json.loads((ROOT/'examples/synthetic-provider-parse.json').read_text()),
            (ROOT/'examples/synthetic-source.txt').read_text())


def test_pipeline_preserves_source_structure_and_excludes_candidate_pay():
    data, source = example()
    original = copy.deepcopy(data)
    result = postprocess_parse(data, source)
    assert data == original
    assert result['fidelity']['ok']
    assert len(result['data']['work_experiences']) == 2
    role = result['data']['work_experiences'][0]['roles'][0]
    assert role['bullets'][1]['heading'] == 'Project: Disaster Recovery Automation'
    assert 'Supported payroll application infrastructure.' in role['bullets']
    assert result['summary_pay_removed'] == 1
    assert all('salary' not in x.lower() for x in result['data']['summary_bullets'])


def test_nested_word_cell_and_textbox_wrapper_are_visible_to_extractor():
    document = Document()
    cell = document.add_table(rows=1, cols=1).cell(0, 0)
    cell.paragraphs[0].text = 'Example Company'
    inner = cell.add_table(rows=1, cols=1)
    inner.cell(0, 0).paragraphs[0].text = 'Jun 2022 to Present'
    inner.cell(0, 0).add_paragraph('Managed fictional infrastructure.')
    wrapper = etree.Element('{'+NS['w']+'}sdt')
    content = etree.SubElement(wrapper, '{'+NS['w']+'}sdtContent')
    for paragraph in list(inner.cell(0, 0)._tc):
        if paragraph.tag == '{'+NS['w']+'}p':
            inner.cell(0, 0)._tc.remove(paragraph)
            content.append(paragraph)
    inner.cell(0, 0)._tc.append(wrapper)
    raw = io.BytesIO()
    document.save(raw)
    text = _extract_docx_text_preserve_tables(raw.getvalue())
    assert 'Example Company' in text
    assert 'Jun 2022 to Present' in text
    assert 'Managed fictional infrastructure.' in text


def test_sanitized_template_has_only_summary_placeholder_text():
    with zipfile.ZipFile(ROOT/'engine/template.docx') as archive:
        texts = []
        for name in archive.namelist():
            if name.endswith('.xml'):
                tree = etree.fromstring(archive.read(name))
                texts += tree.xpath('//w:t/text()', namespaces=NS)
        assert set(value for value in texts if value.strip()) == {'*Summary*', 'Private & Confidential | © Hyppies.com'}


def test_renderer_keeps_summary_roles_project_and_education(tmp_path):
    data, source = example()
    result = postprocess_parse(data, source)
    prepared = prepare_export(result['data'], bullet_levels=result['bullet_levels'])
    output = tmp_path/'example.docx'
    render(prepared, output, work_folder=tmp_path)
    with zipfile.ZipFile(output) as archive:
        for name in archive.namelist():
            if name.endswith('.xml'):
                etree.fromstring(archive.read(name))
        tree = etree.fromstring(archive.read('word/document.xml'))
        text = ' '.join(tree.xpath('//w:t/text()', namespaces=NS))
        assert 'Avery Example'.lower() in text.lower()
        for required in ['Example Cloud', 'Example Systems', 'Disaster Recovery Automation', 'CGPA 3.70/4.00', 'Computer Systems']:
            assert required in text
        assert 'Expected salary' not in text
        extracted = _extract_docx_text_preserve_tables(output.read_bytes())
        assert 'Example Cloud' in extracted
        textbox_text = '\n'.join(_extract_docx_textbox_lines(archive.read('word/document.xml').decode()))
        assert 'cloud operations' in textbox_text
        assert '*Summary*' not in text
        assert '_CVStudioSummaryBox' in archive.read('word/document.xml').decode()
