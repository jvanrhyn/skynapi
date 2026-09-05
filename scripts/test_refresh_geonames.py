import io
from pathlib import Path
import tempfile
import unittest
import zipfile

from refresh_geonames import read_places, write_migration


def place(ident='957654', name='Sandton', code='PPLX', population='0'):
    return [ident, name, name, 'Other name', '-26.104', '28.054', 'P', code,
            'ZA', '', '06', 'JHB', 'JHB', '', population, '', '1626',
            'Africa/Johannesburg', '2024-07-16']


class RefreshTests(unittest.TestCase):
    def parse(self, rows):
        with tempfile.TemporaryDirectory() as temp:
            path = Path(temp) / 'ZA.zip'
            with zipfile.ZipFile(path, 'w') as archive:
                archive.writestr('ZA.txt', '\n'.join('\t'.join(row) for row in rows))
            return read_places(path, 'ZA')

    def test_keeps_zero_population_suburbs_excludes_abandoned_places(self):
        rows = self.parse([place(), place('2', code='PPLQ')])
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0][1], 'Sandton')

    def test_rejects_invalid_data_before_emitting_sql(self):
        for field, value in [(4, 'nan'), (5, '181'), (8, 'US'), (14, '-1'), (18, 'bad')]:
            with self.subTest(field=field):
                row = place(); row[field] = value
                with self.assertRaises(ValueError):
                    self.parse([row])
        with self.assertRaises(ValueError):
            self.parse([place(), place()])
        with self.assertRaises(ValueError):
            self.parse([place()[:-1]])

    def test_upsert_escapes_names_and_preserves_newer_data(self):
        rows = self.parse([place(name="O'Brien\\town")])
        sql = io.StringIO()
        write_migration(sql, rows, 'ZA', 'test-sha')
        result = sql.getvalue()
        self.assertIn("E'O''Brien\\\\town'", result)
        self.assertIn('ON CONFLICT (geonameid) DO UPDATE', result)
        self.assertIn('EXCLUDED.modification_date >= all_countries.modification_date', result)
        self.assertIn('BEGIN;', result)
        self.assertTrue(result.endswith('COMMIT;\n'))

    def test_empty_or_wrong_country_archive_is_rejected(self):
        with self.assertRaises(ValueError):
            self.parse([])
        row = place(); row[8] = 'GB'
        with self.assertRaises(ValueError):
            self.parse([row])


if __name__ == '__main__':
    unittest.main()
