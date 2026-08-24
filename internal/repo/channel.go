package repo

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"observatory/internal/apperr"
	"observatory/internal/model"
	"observatory/internal/store/sqlite"
)

// ChannelRepo 为探测器通道仓储。
type ChannelRepo struct{}

// NewChannelRepo 创建通道仓储。
func NewChannelRepo() *ChannelRepo { return &ChannelRepo{} }

// Create 插入通道。
func (r *ChannelRepo) Create(ctx context.Context, q sqlite.Querier, c *model.DetectorChannel, now time.Time) error {
	res, err := q.ExecContext(ctx,
		`INSERT INTO detector_channels(instrument_id,channel_no,name,wavelength_nm,gain,offset,status,version,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,1,?,?)`,
		c.InstrumentID, c.ChannelNo, c.Name, c.WavelengthNM, c.Gain, c.Offset, c.Status, ts(now), ts(now))
	if err != nil {
		if IsUniqueViolation(err) {
			return apperr.New(apperr.CodeDuplicate, "该仪器下通道号已存在").
				WithDetail("channel_no", c.ChannelNo)
		}
		return err
	}
	c.ID, err = lastID(res)
	c.Version = 1
	c.CreatedAt, c.UpdatedAt = now, now
	return err
}

func scanChannel(row interface{ Scan(...any) error }) (*model.DetectorChannel, error) {
	var c model.DetectorChannel
	var created, updated string
	err := row.Scan(&c.ID, &c.InstrumentID, &c.ChannelNo, &c.Name, &c.WavelengthNM,
		&c.Gain, &c.Offset, &c.Status, &c.Version, &created, &updated)
	if err != nil {
		return nil, err
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if c.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &c, nil
}

// Get 按 id 取通道。
func (r *ChannelRepo) Get(ctx context.Context, q sqlite.Querier, id int64) (*model.DetectorChannel, error) {
	row := q.QueryRowContext(ctx,
		`SELECT id,instrument_id,channel_no,name,wavelength_nm,gain,offset,status,version,created_at,updated_at
		 FROM detector_channels WHERE id=?`, id)
	c, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.NotFound("探测器通道", id)
	}
	return c, err
}

// ListByInstrument 分页列出仪器通道。
func (r *ChannelRepo) ListByInstrument(ctx context.Context, q sqlite.Querier, instrumentID int64, page Page) ([]model.DetectorChannel, error) {
	page = page.Normalize()
	rows, err := q.QueryContext(ctx,
		`SELECT id,instrument_id,channel_no,name,wavelength_nm,gain,offset,status,version,created_at,updated_at
		 FROM detector_channels WHERE instrument_id=? AND id > ? ORDER BY id LIMIT ?`,
		instrumentID, page.Cursor, page.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DetectorChannel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// ListAllByInstrument 列出仪器全部通道（快照用）。
func (r *ChannelRepo) ListAllByInstrument(ctx context.Context, q sqlite.Querier, instrumentID int64) ([]model.DetectorChannel, error) {
	return r.ListByInstrument(ctx, q, instrumentID, Page{Limit: 100})
}

// Update 乐观锁更新通道配置与状态。
func (r *ChannelRepo) Update(ctx context.Context, q sqlite.Querier, c *model.DetectorChannel, now time.Time) error {
	err := execOptimistic(ctx, q, "detector_channels", "探测器通道", c.ID,
		`UPDATE detector_channels SET name=?, wavelength_nm=?, gain=?, offset=?, status=?,
		 version=version+1, updated_at=? WHERE id=? AND version=?`,
		c.Name, c.WavelengthNM, c.Gain, c.Offset, c.Status, ts(now), c.ID, c.Version)
	if err != nil {
		return err
	}
	c.Version++
	c.UpdatedAt = now
	return nil
}
