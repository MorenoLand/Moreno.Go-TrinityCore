-- Tiger Matriarch Credit (Durotar, Echo Isles) --
-- Lua port of npc_tiger_matriarch_creditAI in
-- src/server/scripts/Kalimdor/zone_durotar.cpp (lines 119-181).
-- Entry (wowhead.com/cata/npc=40301 + wowhead.com/mop-classic/npc=40301,
-- verified 2026-11-06; no wowhead.com/classic page — the Cata-era
-- "Lady Of Da Tigers" content lives in this TC tree, so the tree's own
-- script name is the authority and both Classic-family DBs agree on
-- 40301): 40301.
-- Ported arms (C++-exact):
-- - Ctor: SetCombatMovement(false) + EVENT_CHECK_SUMMON_AURA at 2s.
--   In Go the ctor arm lands on ev5 (spawn); the pump is a one-shot
--   chain (fizzule idiom): first check at 2000ms, re-arm 5000ms after
--   every check, exactly the C++ cadence. Cancelled on ev4 (died);
--   respawn re-arms via ev5. No Reset override exists in C++, so the
--   pump is NOT cancelled on ev23 (the C++ EventMap survives Reset).
-- - UpdateAI check (every tick): GetCreaturesInGrid(NPC_TIGER_VEHICLE
--   40305, 15.0f) — 2D metric, GUID-sorted (lua_creature_events.go:5702;
--   the C++ grid-visit order is unspecified); for each tiger: skip
--   unless IsSummon (motion.IsTempSummon, :872); resolve the summoner
--   via GetSummonerUnit (:4874); if the summoner lacks
--   SPELL_NO_SUMMON_AURA 75213 and SPELL_SUMMON_ZENTABRA_TRIGGER 75212
--   and is not in combat, apply both auras to the summoner
--   (player:AddAura twin of me->AddAura(spell, summoner)),
--   have the summoner triggered-cast SPELL_SUMMON_MATRIARCH 75187 on
--   itself (player:CastSpell rides the triggered pipeline), and
--   Talk(SAY_MATRIARCH_AGGRO 0) whispered to the summoner.
-- Deliberate deviations (await engine bridges):
-- - SPELL_SUMMON_MATRIARCH 75187 is a summon-effect cast on the player
--   path, which currently fires only the SMSG_SPELL_GO visual and
--   spawns nothing (lua_player.go CastSpell comment) — the matriarch
--   itself appears only once the session path gains a summon executor.
--   The aura + Talk + cast-visual legs are live; the summon leg is
--   the engine gap, not a script gap.
-- - The sibling npc_tiger_matriarch (entry 40312, cata/mop-verified)
--   stays out: IsSummonedBy rides summoner->GetVehicle() (no bridge),
--   and its pounce/nosummon timers are gated on the vehicle-derived
--   _tigerGuid — porting them alone would fabricate casts.
-- The spell_mount_check / spell_voljin_war_drums / spell_voodoo
-- SpellScripts in the same file stay out (no SpellScript bridge).

local CREDIT_ENTRY = 40301

local NPC_TIGER_VEHICLE = 40305

local SPELL_SUMMON_MATRIARCH = 75187
local SPELL_NO_SUMMON_AURA = 75213
local SPELL_DETECT_INVIS = 75180
local SPELL_SUMMON_ZENTABRA_TRIGGER = 75212

local SAY_MATRIARCH_AGGRO = 0

local timers = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function creditCheck(creature, guid)
    local tigers = creature:GetCreaturesInGrid(NPC_TIGER_VEHICLE, 15.0)
    if tigers ~= nil then
        for _, tiger in ipairs(tigers) do
            if tiger:IsSummon() then
                local summoner = tiger:GetSummonerUnit()
                if summoner ~= nil
                    and not summoner:HasAura(SPELL_NO_SUMMON_AURA)
                    and not summoner:HasAura(SPELL_SUMMON_ZENTABRA_TRIGGER)
                    and not summoner:IsInCombat() then
                    summoner:AddAura(SPELL_NO_SUMMON_AURA)
                    summoner:AddAura(SPELL_DETECT_INVIS)
                    summoner:CastSpell(summoner, SPELL_SUMMON_MATRIARCH, true)
                    creature:Talk(SAY_MATRIARCH_AGGRO, summoner)
                end
            end
        end
    end
    schedule(guid, "check", 5000, function()
        creditCheck(creature, guid)
    end)
end

local function creditSpawn(_, creature)
    creature:SetCombatMovement(false)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "check", 2000, function()
        creditCheck(creature, guid)
    end)
end

local function creditDied(_, creature)
    cancelTimers(creature:GetGUIDLow())
end

RegisterCreatureEvent(CREDIT_ENTRY, 5, creditSpawn)
RegisterCreatureEvent(CREDIT_ENTRY, 4, creditDied)
