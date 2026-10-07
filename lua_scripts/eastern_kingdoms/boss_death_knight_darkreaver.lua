-- Death Knight Darkreaver (14516), Scholomance — Lua port of
-- src/server/scripts/EasternKingdoms/Scholomance/
-- boss_death_knight_darkreaver.cpp (creature AI only); entry has no NPC_
-- constant in scholomance.h (DB-side ScriptName binding, wowhead
-- TBC-cited 14516; EK loader decl :110 / call :288 follows gandling).
-- The C++ AI has no timers and no Talk lines: Reset() and
-- JustEngagedWith() are both empty, and the only arm is DamageTaken.
-- Eluna creature event 9 is the pre-damage hook (thalnos/azshir damaged
-- class), so the lethal-blow check fires exactly where the C++ arm
-- does (Unit::DealDamage fires AI DamageTaken before ModifyHealth).
--
-- Modeled arms (C++-exact where bridges exist):
-- DamageTaken: if current health <= incoming damage (a lethal blow,
-- C++ `me->GetHealth() <= damage` — NOT the golemagg HealthBelowPct
-- bug class, the damage is compared raw), DoCast(me, 23261, true) ->
-- triggered self-cast of 23261 (Summon Darkreaver's Fallen Charger)
-- -> creature:CastSpell(creature, 23261, true).
--
-- Documented-unmodeled (no bridges): none beyond the standard legs —
-- there are no UNIT_STATE_CASTING gates, no instance bookkeeping, and
-- the summon itself (spell 23261's TempSummon leg) is engine-side; the
-- port models only the cast trigger exactly as C++ issues it.

local ENTRY_DARKREAVER = 14516

local SPELL_SUMMON_FALLEN_CHARGER = 23261

-- C++ DamageTaken arm: on a lethal blow (current health <= incoming
-- damage, checked pre-damage), triggered self-cast 23261. No once-guard
-- in C++; the arm can only fire on the killing blow anyway.
local function darkreaverDamageTaken(event, creature, attacker, damage)
    if creature:GetHealth() <= damage then
        creature:CastSpell(creature, SPELL_SUMMON_FALLEN_CHARGER, true)
    end
end

-- C++ Reset and JustEngagedWith are empty — no handlers needed.

RegisterCreatureEvent(ENTRY_DARKREAVER, 9, darkreaverDamageTaken)
