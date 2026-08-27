-- Fixture characters for tools/verify.sh.
--
-- Deliberately uneven: one character who has never logged in, one online, one
-- with master levels and capped jobs, one fresh. That shape is what shakes out
-- divide-by-zero, NULL-join and empty-board bugs on the real pages.

INSERT INTO chars (charid, accid, charname, nation, pos_zone, playtime, timecreated, last_logout) VALUES
  (1, 1, 'Aldwyn',   0, 230, 1487520, '2024-03-11 20:14:00', '2026-08-26 09:41:00'),
  (2, 1, 'Muunbeam', 2, 241,  623040, '2024-07-02 18:02:00', '2026-08-24 23:12:00'),
  (3, 2, 'Krogthar',  1, 234,  204120, '2025-11-19 12:30:00', '2026-08-19 02:55:00'),
  (4, 2, 'Sylvenne', 0, 100,    5400, '2026-08-25 21:00:00', '2026-08-25 22:30:00'),
  (5, 3, 'Neverwas',  1, 234,       0, '2026-08-26 08:00:00', '2026-08-26 08:00:00'),
  -- An unfinished slot: reserved, never named. Must not appear anywhere.
  (6, 3, '',          1, 234,       0, '2026-08-26 09:00:00', '2026-08-26 09:00:00');

INSERT INTO char_stats (charid, mjob, sjob, mlvl, slvl, death, title, master_level, exemplar_points) VALUES
  (1, 12, 13, 99, 49, 0, 145, 12, 184320),
  (2,  3, 20, 99, 49, 0,  38,  4,  22100),
  (3,  1,  6, 76, 38,  0,   0,  0,      0),
  (4,  5,  0, 14,  0,   0,   0,  0,      0),
  (5,  0,  0,  1,  0,   0,   0,  0,      0);

-- char_look is inserted with its model ids in the appearance block below.

INSERT INTO char_history
  (charid, enemies_defeated, times_knocked_out, mh_entrances, joined_parties,
   joined_alliances, spells_cast, abilities_used, ws_used, items_used,
   chats_sent, npc_interactions, battles_fought, gm_calls, distance_travelled) VALUES
  (1, 148902, 812,  941, 1204, 118,  31204, 48210, 96411, 12048, 8241, 6120, 41208, 2, 18402913),
  (2,  61240, 214,  612,  980,  74, 204118,  9820, 12048, 30412, 21048, 4120, 20418, 0,  9120488),
  (3,  22418,  96,  188,  310,  12,   4120, 12048, 18402,  2048,  912, 1840,  9120, 1,  3120884),
  (4,    412,   3,   14,    8,   0,    918,   204,   112,   184,  212,  388,   204, 0,   184029),
  (5,      0,   0,    0,    0,   0,      0,     0,     0,     0,    0,    0,     0, 0,        0);

-- Aldwyn is a job collector, Muunbeam a mage main, Krogthar mid-grind.
INSERT INTO char_jobs (charid, unlocked, genkai, war, mnk, whm, blm, rdm, thf, pld, drk, bst, brd, rng, sam, nin, drg, smn, blu, cor, pup, dnc, sch, geo, run) VALUES
  (1, 2097151, 99, 99, 99, 99, 99, 99, 99, 99, 99, 75, 49, 99, 99, 99, 99, 37, 99, 99, 21, 99, 99, 99, 99),
  (2, 2097151, 99, 49, 30, 99, 99, 99, 51, 12,  0,  0, 99, 20, 20, 49,  0, 99, 30,  0,  0, 49, 99, 99, 12),
  (3, 2097151, 76, 76, 40, 24, 18, 30, 38,  0,  0,  0,  0, 12, 20, 37,  0,  0,  0,  0,  0, 15,  0,  0,  0),
  (4,     126, 50,  6,  0,  9,  4, 14,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0),
  (5,     126, 50,  1,  1,  1,  1,  1,  1,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0,  0);

INSERT INTO char_exp (charid, mode, sam, nin, whm, sch, war, thf, rdm, merits, limits) VALUES
  (1, 0, 41200, 18400,     0,     0,  2100,  8400,     0, 28, 41200),
  (2, 0,     0,  9100, 38200, 12400,     0,     0,  2200, 11,  9820),
  (3, 0,     0,  4200,     0,     0, 21400,  1200,     0,  0,     0),
  (4, 0,     0,     0,     0,     0,     0,     0,   840,  0,     0),
  (5, 0,     0,     0,     0,     0,     0,     0,     0,  0,     0);

INSERT INTO char_job_points (charid, jobid, capacity_points, job_points, job_points_spent) VALUES
  (1, 12, 8400, 1204, 980), (1, 13, 2100, 412, 380), (1, 1, 5200, 640, 600),
  (2,  3, 3200,  480, 420), (2, 20, 1100, 190, 120),
  (3,  1,  420,   12,   0);

INSERT INTO char_profile
  (charid, rank_points, rank_sandoria, rank_bastok, rank_windurst,
   fame_sandoria, fame_bastok, fame_windurst, fame_jeuno, fame_norg, fame_adoulin) VALUES
  (1, 42180, 10, 6, 4, 9999, 4120, 2100, 8400, 1200, 3200),
  (2, 18400,  3, 2, 10, 1100,  820, 9999, 4120,  200, 1800),
  (3,  6200,  1, 7, 1,   410, 6120,  180, 1200,    0,    0),
  (4,   180,  2, 1, 1,   120,    0,    0,    0,    0,    0),
  (5,     0,  1, 1, 1,     0,    0,    0,    0,    0,    0);

INSERT INTO char_points
  (charid, guild_fishing, guild_woodworking, guild_smithing, guild_goldsmithing,
   guild_weaving, guild_leathercraft, guild_bonecraft, guild_alchemy, guild_cooking) VALUES
  (1, 41200, 8400, 62100, 12400, 3200, 9100, 1800, 22400, 51200),
  (2,  1200,  400,     0,     0, 8400,  200,    0,  4100, 12800),
  (3,     0,    0,  2100,     0,    0,    0,    0,     0,     0),
  (4,     0,    0,     0,     0,    0,    0,    0,     0,   180),
  (5,     0,    0,     0,     0,    0,    0,    0,     0,     0);

-- Skill values are stored at ten times the number the game shows.
INSERT INTO char_skills (charid, skillid, value, rank) VALUES
  (1, 10, 4240, 1), (1,  9, 3980, 3), (1, 29, 3680, 7), (1, 31, 3730, 6),
  (1, 39, 3340, 9), (1, 50, 812, 8), (1, 56, 964, 9), (1, 48, 524, 5),
  (2, 33, 4170, 2), (2, 32, 3880, 5), (2, 34, 4040, 3), (2, 36, 4170, 2),
  (2, 12, 2650, 11), (2, 29, 3000, 10), (2, 52, 718, 6), (2, 56, 612, 9),
  (3,  5, 2840, 1), (3,  3, 2610, 3), (3, 30, 2280, 12), (3, 29, 2400, 7),
  (3, 50, 214, 8),
  (4,  2,  480, 3), (4, 36,  520, 2), (4, 56, 124, 9);

-- Gil lives in the inventory at location 0, slot 0.
INSERT INTO char_inventory (charid, location, slot, itemId, quantity) VALUES
  (1, 0, 0, 65535, 48210934),
  (2, 0, 0, 65535,  6120488),
  (3, 0, 0, 65535,   412088),
  (4, 0, 0, 65535,    18402);

-- A real chars.missions blob for charid 1: 15 records of 70 bytes each.
-- Exercises every branch of the decoder: a nation log mid-chain (where 65535
-- would mean none), an expansion log sitting at 0 (where it does mean none),
-- a ToAU mission in progress, CoP whose completion is linear off current, and
-- SoA whose mission ids run past the 64-slot complete array.
UPDATE chars SET missions = UNHEX('17000000000001010101010101010101010101010101010101010101010000000000000000000000000000000000000000000000000000000000000000000000000000000000FFFF0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000FFFF0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000010000000100010001000100010001000100010001000101010001010100010100000000000000000000000000000000000000000000000000000000000000002900000000000101010101010101010101010101010101010101010101010101010101010101010101010101010101000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000B60100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000460000000000010100010001010101010001010101010101010101010001000001010001010100000101000101010101010101010101010101010101000101010101000101010000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000') WHERE charid = 1;

-- Appearance. char_look holds MODEL ids directly.
INSERT INTO char_look (charid, face, race, size, head, body, hands, legs, feet, main, sub, ranged) VALUES
  (1, 3, 3, 1,  98,  57,  57,  57,  57, 20, 0, 0),
  (2, 1, 6, 0,  12,  61,  61,  61,  61, 18, 0, 0),
  (3, 0, 8, 2,   0,  20,  20,  20,  20,  6, 0, 0),
  (4, 5, 7, 1,   0,   8,   8,   8,   8,  2, 0, 0),
  (5, 0, 1, 1,   0,   8,   8,   8,   8,  0, 0, 0),
  (6, 0, 3, 1,   0,   8,   8,   8,   8,  0, 0, 0);

-- char_style holds ITEM ids, which only become models via item_equipment.MId.
-- Aldwyn is style-locked below, Muunbeam is not, so the two rows prove the
-- resolution picks the right source per character.
-- Real items, so the item-id to model-id hop is exercised against the real
-- item_equipment data: hrafn_coronet is MId 323, hexed_haubert is MId 5.
INSERT INTO char_style (charid, head, body, hands, legs, feet, main, sub, ranged) VALUES
  (1, 10400, 10240, 0, 0, 0, 0, 0, 0),
  (2, 10384,     0, 0, 0, 0, 0, 0, 0);

UPDATE chars SET isstylelocked = 1 WHERE charid = 1;

-- Muunbeam is logged in right now.
INSERT INTO accounts_sessions (accid, charid, targid) VALUES (1, 2, 100);
