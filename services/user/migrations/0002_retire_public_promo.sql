-- AURALIS-PREMIUM was seeded with 100,000 redemptions and printed on the
-- premium page, so anyone could get a year of premium. Turn it off; admins
-- grant premium through /admin/users/{id}/entitlement or create real codes.
UPDATE promo_codes SET active = false WHERE code = 'AURALIS-PREMIUM';
