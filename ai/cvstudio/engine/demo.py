"""Run the fictional parse/normalize/render example without an AI provider."""
from pathlib import Path
import argparse
import json
from portable_pipeline import postprocess_parse, prepare_export, render


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', type=Path, default=Path('demo-output'))
    args = parser.parse_args()
    examples = Path(__file__).resolve().parents[1] / 'examples'
    provider_json = json.loads((examples / 'synthetic-provider-parse.json').read_text(encoding='utf-8'))
    source = (examples / 'synthetic-source.txt').read_text(encoding='utf-8')
    result = postprocess_parse(provider_json, source)
    args.output.mkdir(parents=True, exist_ok=True)
    (args.output / 'normalized-result.json').write_text(json.dumps(result, indent=2, ensure_ascii=False), encoding='utf-8')
    data = prepare_export(result['data'], bullet_levels=result['bullet_levels'])
    render(data, args.output / 'formatted-example.docx', work_folder=args.output)
    print('Fictional example complete. No provider requests were made.')
    print('Fidelity passed:', result['fidelity']['ok'])
    print('Candidate-pay summary statements removed:', result['summary_pay_removed'])


if __name__ == '__main__':
    main()
