-- Argent Valiant (Icecrown) --
-- Lua port of src/server/scripts/Northrend/zone_icecrown.cpp
-- (npc_argent_valiantAI — DamageTaken duel-end machine only).
-- Entry: 33448 'Argent Valiant' (TDB-verified, ScriptName
-- npc_argent_valiant; summon-only via the Aspirant's Challenge,
-- zero static spawns).
-- C++ DamageTaken (:75-86): lethal damage (uiDamage > GetHealth)
-- from a player -> damage = 0, the player self-casts
-- SPELL_KILL_CREDIT 63049 (triggered), SetFaction(FACTION_FRIENDLY
-- 35), DespawnOrUnsummon(5s), SetHomePosition(current X/Y/Z/O),
-- EnterEvadeMode().
-- Reset :62 is Initialize-only for the unported UpdateAI combat
-- rotation (63010 charge / 65147 shield-breaker) — stays out. The
-- ctor MovePoint + SetFaction(FRIENDLY) arm and the MovementInform
-- faction flip (:140-147) are out of scope (ctor/MovementInform).
-- Eluna creature event: 9 OnDamageTaken — fireCreatureDamageTaken
-- threads the handler's returns[1] back as the new damage
-- (dummy.lua `return true, 0` precedent). C++ DamageTaken fires
-- pre-death on the killing blow, so the ev9 rewrite (damage = 0)
-- reproduces it — no postDamage > 0 guard (that guard is for
-- UpdateAI HealthBelowPct arms, not DamageTaken).
-- DespawnOrUnsummon(5s) ports as a one-shot CreateLuaEvent 5000ms
-- -> Despawn() (creature:Despawn takes no delay arg).
-- Bridges: player:CastSpell lua_player.go:3501 (cocoon precedent);
-- SetFaction lua_creature_events.go:6698; SetHomePosition :7048;
-- GetX/Y/Z/GetO via the object.go generic fallback (:196, GetO =
-- Orientation field); creature:GetHealth object.go:203;
-- EnterEvadeMode :5624; attacker:GetTypeId() == 4 TYPEID_PLAYER
-- (lecraft precedent).

local ARGENT_VALIANT_ENTRY = 33448
local SPELL_KILL_CREDIT = 63049
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231

local function onDamageTaken(event, creature, attacker, damage)
    if damage > creature:GetHealth() and attacker and attacker:GetTypeId() == 4 then
        attacker:CastSpell(attacker, SPELL_KILL_CREDIT, true)
        creature:SetFaction(FACTION_FRIENDLY)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 5000)
        creature:SetHomePosition(creature:GetX(), creature:GetY(), creature:GetZ(), creature:GetO())
        creature:EnterEvadeMode()
        return true, 0
    end
end

RegisterCreatureEvent(ARGENT_VALIANT_ENTRY, 9, onDamageTaken)
