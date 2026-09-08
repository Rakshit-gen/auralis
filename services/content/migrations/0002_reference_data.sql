-- Genres and languages. These are platform reference data, not seed content,
-- so they ship as a migration and are always present.

INSERT INTO languages (code, name) VALUES
    ('en', 'English'),
    ('hi', 'Hindi'),
    ('es', 'Spanish'),
    ('fr', 'French'),
    ('pt', 'Portuguese'),
    ('de', 'German')
ON CONFLICT (code) DO NOTHING;

INSERT INTO genres (slug, name, description) VALUES
    ('mystery', 'Mystery', 'Investigations, missing pieces, and answers that cost something.'),
    ('science-fiction', 'Science Fiction', 'Near and far futures, and the rules that hold them together.'),
    ('thriller', 'Thriller', 'Pressure that does not let up until the last minute.'),
    ('drama', 'Drama', 'People, decisions, and the weight they carry afterward.'),
    ('folk-horror', 'Folk Horror', 'Places that remember, and traditions that were never really abandoned.'),
    ('adventure', 'Adventure', 'Movement, stakes, and somewhere that has to be reached.'),
    ('noir', 'Noir', 'Bad options, worse people, and a narrator who has stopped pretending.'),
    ('romance', 'Romance', 'Two people, the space between them, and whether it closes.'),
    ('history', 'History', 'Real periods, fictional lives, carefully kept apart from the record.'),
    ('comedy', 'Comedy', 'Timing, and the relief of being allowed to laugh.'),
    ('fantasy', 'Fantasy', 'Systems of magic with a price, and worlds that run on them.'),
    ('slice-of-life', 'Slice of Life', 'Small scale, close attention, nothing exploding.')
ON CONFLICT (slug) DO NOTHING;
